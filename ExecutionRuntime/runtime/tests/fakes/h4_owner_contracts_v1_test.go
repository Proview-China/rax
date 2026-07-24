package fakes_test

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/runtime/conformance"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/fakes"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/ports"
)

func TestBindingAdmissionV1LostReplyInspectAndConcurrent64(t *testing.T) {
	now := time.Unix(191_000, 0)
	request, _, _ := h4FixtureV1(t, now)
	store := fakes.NewBindingAdmissionStoreV1(func() time.Time { return now })
	store.InjectLostStartReplyV1()
	if _, err := store.StartOrInspectBindingAdmissionV1(context.Background(), request); !core.HasReason(err, core.ReasonInspectCoverageIncomplete) {
		t.Fatalf("expected injected lost reply, got %v", err)
	}
	recovered, err := store.InspectBindingAdmissionV1(context.Background(), ports.BindingAdmissionInspectRequestV1{AttemptID: request.AttemptID, RequestDigest: request.RequestDigest})
	if err != nil {
		t.Fatal(err)
	}
	if err := recovered.ValidateCurrent(request, now); err != nil {
		t.Fatal(err)
	}
	const workers = 64
	var successes atomic.Int64
	var first core.Digest
	var firstMu sync.Mutex
	var wait sync.WaitGroup
	wait.Add(workers)
	for index := 0; index < workers; index++ {
		go func() {
			defer wait.Done()
			result, callErr := store.StartOrInspectBindingAdmissionV1(context.Background(), request)
			if callErr != nil {
				t.Errorf("concurrent Binding admission failed: %v", callErr)
				return
			}
			firstMu.Lock()
			if first == "" {
				first = result.ResultDigest
			} else if first != result.ResultDigest {
				t.Errorf("concurrent Binding admission returned another result")
			}
			firstMu.Unlock()
			successes.Add(1)
		}()
	}
	wait.Wait()
	if successes.Load() != workers || store.CommitCountV1() != 1 {
		t.Fatalf("Binding admission did not linearize one result: successes=%d commits=%d", successes.Load(), store.CommitCountV1())
	}
	drifted := request
	drifted.ExpectedBindingSetID = "binding-set-other"
	drifted.RequestDigest = ""
	drifted, err = ports.SealBindingAdmissionRequestV1(drifted)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.StartOrInspectBindingAdmissionV1(context.Background(), drifted); !core.HasReason(err, core.ReasonIdempotencyPayloadMismatch) {
		t.Fatalf("same Attempt accepted changed content: %v", err)
	}
}

func TestH4OwnerCurrentStoresAndConformanceRemainReferenceOnly(t *testing.T) {
	t.Parallel()
	now := time.Unix(191_100, 0)
	request, availability, resources := h4FixtureV1(t, now)
	store := fakes.NewH4OwnerCurrentStoreV1()
	for _, binding := range resources.Bindings {
		handle := h4HandleV1(t, now, binding.Handle.ID, binding.ComponentID)
		if _, err := store.EnsureResourceHandleCurrentV1(context.Background(), handle); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := store.EnsureResourceBindingSetCurrentV1(context.Background(), resources); err != nil {
		t.Fatal(err)
	}
	if _, err := store.EnsureAgentExecutionAvailabilityV1(context.Background(), availability); err != nil {
		t.Fatal(err)
	}
	availabilityReport, err := conformance.CheckAgentExecutionAvailabilityV1(context.Background(), conformance.AgentExecutionAvailabilityCaseV1{Reader: store, Ref: availability.Ref, Now: now})
	if err != nil {
		t.Fatal(err)
	}
	resourceReport, err := conformance.CheckResourceBindingV1(context.Background(), conformance.ResourceBindingCaseV1{Reader: store, Set: resources.Ref, Now: now})
	if err != nil {
		t.Fatal(err)
	}
	admissions := fakes.NewBindingAdmissionStoreV1(func() time.Time { return now })
	admissionReport, err := conformance.CheckBindingAdmissionV1(context.Background(), conformance.BindingAdmissionCaseV1{Gateway: admissions, Request: request, Now: now})
	if err != nil {
		t.Fatal(err)
	}
	if !availabilityReport.ExactCurrentObserved || !availabilityReport.ReadyEpochObserved || availabilityReport.ProductionClaimEligible || !resourceReport.SetCurrentObserved || !resourceReport.AllHandlesCurrent || resourceReport.ProductionClaimEligible || !admissionReport.StartOrInspectObserved || !admissionReport.ExactInspectObserved || !admissionReport.PreBindingOnly || admissionReport.ProductionClaimEligible {
		t.Fatalf("H4 conformance report drifted or claimed production: availability=%+v resources=%+v admission=%+v", availabilityReport, resourceReport, admissionReport)
	}
}

func TestAgentExecutionAvailabilityV1ConcurrentFenceRejectsOldEpoch(t *testing.T) {
	now := time.Unix(191_200, 0)
	_, ready, _ := h4FixtureV1(t, now)
	store := fakes.NewH4OwnerCurrentStoreV1()
	if _, err := store.EnsureAgentExecutionAvailabilityV1(context.Background(), ready); err != nil {
		t.Fatal(err)
	}
	fenced, err := ports.SealAgentExecutionAvailabilityProjectionV1(ports.AgentExecutionAvailabilityProjectionV1{
		Ref:    ports.AgentExecutionAvailabilityRefV1{Owner: ready.Ref.Owner, ID: ready.Ref.ID, Revision: 2, Epoch: ready.Ref.Epoch},
		HostID: ready.HostID, StartID: ready.StartID, SystemReady: ready.SystemReady, State: ports.AgentExecutionAvailabilityFencedV1,
		CheckedUnixNano: now.Add(time.Second).UnixNano(), ExpiresUnixNano: ready.ExpiresUnixNano,
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.EnsureAgentExecutionAvailabilityV1(context.Background(), fenced); err != nil {
		t.Fatal(err)
	}
	if _, err := store.InspectAgentExecutionAvailabilityCurrentV1(context.Background(), ready.Ref); !core.HasReason(err, core.ReasonBindingDrift) {
		t.Fatalf("old availability epoch remained exact-current: %v", err)
	}
	current, err := store.InspectAgentExecutionAvailabilityCurrentV1(context.Background(), fenced.Ref)
	if err != nil {
		t.Fatal(err)
	}
	if err := current.ValidateCurrent(fenced.Ref, now.Add(time.Second)); !core.HasReason(err, core.ReasonFencedInstance) {
		t.Fatalf("fenced availability admitted work: %v", err)
	}
}

func h4FixtureV1(t *testing.T, now time.Time) (ports.BindingAdmissionRequestV1, ports.AgentExecutionAvailabilityProjectionV1, ports.ResourceBindingSetV1) {
	t.Helper()
	handle := h4HandleV1(t, now, "resource-runtime", "praxis/runtime")
	resources, err := ports.SealResourceBindingSetV1(ports.ResourceBindingSetV1{
		Ref:             ports.ResourceBindingSetRefV1{ID: "resource-set-a", Revision: 1},
		Bindings:        []ports.ResourceBindingV1{{ComponentID: "praxis/runtime", Handle: handle.Ref, CleanupContract: handle.CleanupContract, DeploymentAttestation: handle.DeploymentAttestation}},
		CheckedUnixNano: now.UnixNano(), ExpiresUnixNano: now.Add(time.Minute).UnixNano(),
	})
	if err != nil {
		t.Fatal(err)
	}
	hostOwner := core.OwnerRef{Domain: "praxis.host", ID: "host-owner"}
	availability, err := ports.SealAgentExecutionAvailabilityProjectionV1(ports.AgentExecutionAvailabilityProjectionV1{
		Ref:    ports.AgentExecutionAvailabilityRefV1{Owner: hostOwner, ID: "availability-a", Revision: 1, Epoch: 1},
		HostID: "host-a", StartID: "start-a", SystemReady: h4OwnerCurrentV1("system-ready", hostOwner, now.Add(2*time.Minute)), State: ports.AgentExecutionAvailabilityReadyV1,
		CheckedUnixNano: now.UnixNano(), ExpiresUnixNano: now.Add(time.Minute).UnixNano(),
	})
	if err != nil {
		t.Fatal(err)
	}
	owner := core.OwnerRef{Domain: "praxis.owner", ID: "owner-a"}
	current := func(id string) ports.OwnerCurrentRefV1 { return h4OwnerCurrentV1(id, owner, now.Add(2*time.Minute)) }
	release := ports.PreBindingComponentReleaseV1{ComponentID: "praxis/runtime", Release: current("release-runtime"), Certification: current("cert-runtime"), DeploymentReadiness: current("deployment-runtime")}
	request, err := ports.SealBindingAdmissionRequestV1(ports.BindingAdmissionRequestV1{
		AttemptID: "binding-attempt-a", DefinitionCurrent: current("definition"), PlanCurrent: current("plan"), AssemblyCurrent: current("assembly"), CatalogCurrent: current("catalog"), ResolutionCurrent: current("resolution"), Releases: []ports.PreBindingComponentReleaseV1{release}, ResourceBindingSet: resources.Ref, AuthorityCurrent: current("authority"), PolicyCurrent: current("policy"), ExpectedBindingSetID: "binding-set-a", RequestedNotAfterUnixNano: now.Add(time.Minute).UnixNano(),
	})
	if err != nil {
		t.Fatal(err)
	}
	return request, availability, resources
}

func h4HandleV1(t *testing.T, now time.Time, id string, component ports.ComponentIDV2) ports.ResourceHandleCurrentV1 {
	t.Helper()
	owner := core.OwnerRef{Domain: "praxis.resource", ID: core.OwnerID("owner-" + id)}
	value, err := ports.SealResourceHandleCurrentV1(ports.ResourceHandleCurrentV1{
		Ref:             ports.ResourceHandleRefV1{Owner: owner, ID: id, Revision: 1, Kind: "praxis/sqlite", ScopeDigest: core.DigestBytes([]byte("scope-" + string(component)))},
		CleanupContract: h4OwnerCurrentV1("cleanup-"+id, owner, now.Add(2*time.Minute)), DeploymentAttestation: h4OwnerCurrentV1("deployment-"+id, owner, now.Add(2*time.Minute)),
		CheckedUnixNano: now.UnixNano(), ExpiresUnixNano: now.Add(time.Minute).UnixNano(),
	})
	if err != nil {
		t.Fatal(err)
	}
	return value
}

func h4OwnerCurrentV1(id string, owner core.OwnerRef, expires time.Time) ports.OwnerCurrentRefV1 {
	return ports.OwnerCurrentRefV1{Owner: owner, ContractVersion: "1.0.0", ID: id, Revision: 1, Digest: core.DigestBytes([]byte(id)), ExpiresUnixNano: expires.UnixNano()}
}

package ports_test

import (
	"encoding/json"
	"reflect"
	"testing"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/ports"
)

func TestAgentExecutionAvailabilityV1CanonicalCurrentAndFence(t *testing.T) {
	t.Parallel()
	now := time.Unix(190_100, 0)
	ready := availabilityProjectionV1(t, now, 1, 1, ports.AgentExecutionAvailabilityReadyV1)
	if err := ready.ValidateCurrent(ready.Ref, now); err != nil {
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
	if err := ports.ValidateAgentExecutionAvailabilityTransitionV1(ready, fenced); err != nil {
		t.Fatal(err)
	}
	if err := fenced.ValidateCurrent(fenced.Ref, now.Add(time.Second)); !core.HasReason(err, core.ReasonFencedInstance) {
		t.Fatalf("fenced epoch remained current: %v", err)
	}
	lateFence, err := ports.SealAgentExecutionAvailabilityProjectionV1(ports.AgentExecutionAvailabilityProjectionV1{
		Ref:             ports.AgentExecutionAvailabilityRefV1{Owner: ready.Ref.Owner, ID: ready.Ref.ID, Revision: 2, Epoch: ready.Ref.Epoch},
		HostID:          ready.HostID,
		StartID:         ready.StartID,
		SystemReady:     ready.SystemReady,
		State:           ports.AgentExecutionAvailabilityFencedV1,
		CheckedUnixNano: ready.SystemReady.ExpiresUnixNano,
		ExpiresUnixNano: ready.SystemReady.ExpiresUnixNano + int64(time.Minute),
	})
	if err != nil {
		t.Fatalf("late terminal fence could not outlive expired ready proof: %v", err)
	}
	if err := ports.ValidateAgentExecutionAvailabilityTransitionV1(ready, lateFence); err != nil {
		t.Fatal(err)
	}
	reopen := availabilityProjectionV1(t, now.Add(2*time.Second), 3, 2, ports.AgentExecutionAvailabilityReadyV1)
	if err := ports.ValidateAgentExecutionAvailabilityTransitionV1(fenced, reopen); !core.HasReason(err, core.ReasonFencedInstance) {
		t.Fatalf("fenced Start reopened: %v", err)
	}
	tampered := ready
	tampered.StartID = "other-start"
	if err := tampered.Validate(); !core.HasReason(err, core.ReasonBindingDrift) {
		t.Fatalf("availability canonical tamper passed: %v", err)
	}
}

func TestResourceBindingV1CanonicalOrderingProofBindingAndTTL(t *testing.T) {
	t.Parallel()
	now := time.Unix(190_200, 0)
	first := resourceHandleV1(t, now, "resource-b", "praxis/runtime")
	second := resourceHandleV1(t, now, "resource-a", "praxis/harness")
	set, err := ports.SealResourceBindingSetV1(ports.ResourceBindingSetV1{
		Ref: ports.ResourceBindingSetRefV1{ID: "resources-a", Revision: 1},
		Bindings: []ports.ResourceBindingV1{
			{ComponentID: "praxis/runtime", Handle: first.Ref, CleanupContract: first.CleanupContract, DeploymentAttestation: first.DeploymentAttestation},
			{ComponentID: "praxis/harness", Handle: second.Ref, CleanupContract: second.CleanupContract, DeploymentAttestation: second.DeploymentAttestation},
		},
		CheckedUnixNano: now.UnixNano(), ExpiresUnixNano: now.Add(time.Minute).UnixNano(),
	})
	if err != nil {
		t.Fatal(err)
	}
	if set.Bindings[0].ComponentID != "praxis/harness" || set.Bindings[1].ComponentID != "praxis/runtime" {
		t.Fatalf("Resource BindingSet was not canonicalized: %+v", set.Bindings)
	}
	if err := set.ValidateCurrent(set.Ref, now); err != nil {
		t.Fatal(err)
	}
	tamperedProof := set
	tamperedProof.Bindings = append([]ports.ResourceBindingV1{}, set.Bindings...)
	tamperedProof.Bindings[0].CleanupContract.ID = "other-cleanup"
	if err := tamperedProof.Validate(); !core.HasReason(err, core.ReasonBindingDrift) {
		t.Fatalf("Resource cleanup proof splice passed: %v", err)
	}
	badTTL := set
	badTTL.Bindings = append([]ports.ResourceBindingV1{}, set.Bindings...)
	badTTL.ExpiresUnixNano = now.Add(3 * time.Minute).UnixNano()
	badTTL.Ref.Digest, badTTL.ProjectionDigest = "", ""
	if _, err := ports.SealResourceBindingSetV1(badTTL); !core.HasReason(err, core.ReasonBindingExpired) {
		t.Fatalf("Resource BindingSet exceeded member TTL: %v", err)
	}
}

func TestBindingAdmissionV1IsCanonicalAndNominallyPreBindingOnly(t *testing.T) {
	t.Parallel()
	now := time.Unix(190_300, 0)
	resources := resourceBindingSetV1(t, now)
	request := bindingAdmissionRequestPortsV1(t, now, resources.Ref)
	if err := request.ValidateCurrent(now); err != nil {
		t.Fatal(err)
	}
	if request.Releases[0].ComponentID != "praxis/context" || request.Releases[1].ComponentID != "praxis/runtime" {
		t.Fatalf("pre-binding releases were not canonicalized: %+v", request.Releases)
	}
	typ := reflect.TypeOf(request)
	for _, forbidden := range []string{"ConstructedComponentCurrent", "ActivationCurrent", "GenerationAssociationCurrent", "ComponentProductionCurrent"} {
		if _, exists := typ.FieldByName(forbidden); exists {
			t.Fatalf("Binding admission imported post-binding field %s", forbidden)
		}
	}
	encoded, err := json.Marshal(request)
	if err != nil {
		t.Fatal(err)
	}
	var object map[string]any
	if err := json.Unmarshal(encoded, &object); err != nil {
		t.Fatal(err)
	}
	object["activation_current"] = map[string]any{"id": "forbidden"}
	encoded, _ = json.Marshal(object)
	var decoded ports.BindingAdmissionRequestV1
	if err := core.DecodeStrictJSON(encoded, &decoded); !core.HasReason(err, core.ReasonInvalidCanonicalForm) {
		t.Fatalf("strict Binding admission accepted post-binding field: %v", err)
	}
	tampered := request
	tampered.ExpectedBindingSetID = "other-set"
	if err := tampered.Validate(); !core.HasReason(err, core.ReasonInvalidDigest) {
		t.Fatalf("Binding admission request digest tamper passed: %v", err)
	}
}

func availabilityProjectionV1(t *testing.T, now time.Time, revision core.Revision, epoch core.Epoch, state ports.AgentExecutionAvailabilityStateV1) ports.AgentExecutionAvailabilityProjectionV1 {
	t.Helper()
	owner := core.OwnerRef{Domain: "praxis.host", ID: "host-owner"}
	ready := ownerCurrentRefV1("system-ready", now.Add(2*time.Minute), owner)
	value, err := ports.SealAgentExecutionAvailabilityProjectionV1(ports.AgentExecutionAvailabilityProjectionV1{
		Ref:    ports.AgentExecutionAvailabilityRefV1{Owner: owner, ID: "availability-a", Revision: revision, Epoch: epoch},
		HostID: "host-a", StartID: "start-a", SystemReady: ready, State: state,
		CheckedUnixNano: now.UnixNano(), ExpiresUnixNano: now.Add(time.Minute).UnixNano(),
	})
	if err != nil {
		t.Fatal(err)
	}
	return value
}

func resourceHandleV1(t *testing.T, now time.Time, id string, component ports.ComponentIDV2) ports.ResourceHandleCurrentV1 {
	t.Helper()
	owner := core.OwnerRef{Domain: "praxis.resource", ID: core.OwnerID("owner-" + id)}
	value, err := ports.SealResourceHandleCurrentV1(ports.ResourceHandleCurrentV1{
		Ref:                   ports.ResourceHandleRefV1{Owner: owner, ID: id, Revision: 1, Kind: "praxis/sqlite", ScopeDigest: digestH4V1("scope-" + id)},
		CleanupContract:       ownerCurrentRefV1("cleanup-"+id, now.Add(2*time.Minute), owner),
		DeploymentAttestation: ownerCurrentRefV1("deployment-"+id, now.Add(2*time.Minute), owner),
		CheckedUnixNano:       now.UnixNano(), ExpiresUnixNano: now.Add(time.Minute).UnixNano(),
	})
	if err != nil {
		t.Fatal(err)
	}
	_ = component
	return value
}

func resourceBindingSetV1(t *testing.T, now time.Time) ports.ResourceBindingSetV1 {
	t.Helper()
	handle := resourceHandleV1(t, now, "resource-runtime", "praxis/runtime")
	value, err := ports.SealResourceBindingSetV1(ports.ResourceBindingSetV1{
		Ref:             ports.ResourceBindingSetRefV1{ID: "resources-runtime", Revision: 1},
		Bindings:        []ports.ResourceBindingV1{{ComponentID: "praxis/runtime", Handle: handle.Ref, CleanupContract: handle.CleanupContract, DeploymentAttestation: handle.DeploymentAttestation}},
		CheckedUnixNano: now.UnixNano(), ExpiresUnixNano: now.Add(time.Minute).UnixNano(),
	})
	if err != nil {
		t.Fatal(err)
	}
	return value
}

func bindingAdmissionRequestPortsV1(t *testing.T, now time.Time, resources ports.ResourceBindingSetRefV1) ports.BindingAdmissionRequestV1 {
	t.Helper()
	owner := core.OwnerRef{Domain: "praxis.owner", ID: "owner-a"}
	current := func(id string) ports.OwnerCurrentRefV1 { return ownerCurrentRefV1(id, now.Add(2*time.Minute), owner) }
	release := func(component ports.ComponentIDV2) ports.PreBindingComponentReleaseV1 {
		return ports.PreBindingComponentReleaseV1{ComponentID: component, Release: current("release-" + string(component)), Certification: current("cert-" + string(component)), DeploymentReadiness: current("deployment-" + string(component))}
	}
	value, err := ports.SealBindingAdmissionRequestV1(ports.BindingAdmissionRequestV1{
		AttemptID: "binding-attempt-a", DefinitionCurrent: current("definition"), PlanCurrent: current("plan"), AssemblyCurrent: current("assembly"), CatalogCurrent: current("catalog"), ResolutionCurrent: current("resolution"),
		Releases: []ports.PreBindingComponentReleaseV1{release("praxis/runtime"), release("praxis/context")}, ResourceBindingSet: resources,
		AuthorityCurrent: current("authority"), PolicyCurrent: current("policy"), ExpectedBindingSetID: "binding-set-a", RequestedNotAfterUnixNano: now.Add(time.Minute).UnixNano(),
	})
	if err != nil {
		t.Fatal(err)
	}
	return value
}

func ownerCurrentRefV1(id string, expires time.Time, owner core.OwnerRef) ports.OwnerCurrentRefV1 {
	return ports.OwnerCurrentRefV1{Owner: owner, ContractVersion: "1.0.0", ID: id, Revision: 1, Digest: digestH4V1(id), ExpiresUnixNano: expires.UnixNano()}
}

func digestH4V1(value string) core.Digest { return core.DigestBytes([]byte(value)) }

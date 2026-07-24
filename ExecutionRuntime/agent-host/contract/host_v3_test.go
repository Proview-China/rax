package contract_test

import (
	"testing"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/agent-host/contract"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	runtimeports "github.com/Proview-China/rax/ExecutionRuntime/runtime/ports"
)

func TestHostV3StartRequestCanonicalClaimInputAndTTL(t *testing.T) {
	now := time.Unix(2_400_000_000, 0)
	request := startRequestContractFixtureV3(t, now)
	input, err := request.ClaimInputV3()
	if err != nil {
		t.Fatal(err)
	}
	if input.DeploymentCurrentRef != request.DeploymentCurrentRef || input.DefinitionSourceRef != request.DefinitionSourceCurrent {
		t.Fatalf("input=%+v", input)
	}
	reordered := request
	reordered.RequestDigest = ""
	reordered.Config.StatePlaneBindings = []string{"state-b", "state-a"}
	reordered, err = contract.SealStartRequestV3(reordered)
	if err != nil || reordered.RequestDigest != request.RequestDigest {
		t.Fatalf("canonical=%v %s/%s", err, reordered.RequestDigest, request.RequestDigest)
	}
	drift := request
	drift.DeploymentCurrentRef.DeploymentID = "deployment-other"
	drift.RequestDigest = ""
	drift, _ = contract.SealStartRequestV3(drift)
	if drift.RequestDigest == request.RequestDigest {
		t.Fatal("deployment drift did not change digest")
	}
	if err = request.ValidateCurrent(now.Add(2 * time.Hour)); !contract.HasCode(err, contract.ErrorPrecondition) {
		t.Fatalf("expired=%v", err)
	}
}

func TestHostV3InspectStopAndResultExactWindows(t *testing.T) {
	now := time.Unix(2_400_000_000, 0)
	start := startRequestContractFixtureV3(t, now)
	input, _ := start.ClaimInputV3()
	claim, _ := input.ClaimV1()
	claimRef, _ := claim.CurrentRefV1()
	closure := exactContractV3(t, "praxis.agent-host/cleanup-closure", "closure-1")
	journal := exactContractV3(t, "praxis.agent-host/journal", "journal-1")
	inspect, err := contract.SealInspectRequestV3(contract.InspectRequestV3{HostID: claim.HostID, StartID: claim.StartID, StartClaim: claimRef, RequestedAtUnixNano: now.UnixNano(), RequestedNotAfterUnixNano: now.Add(20 * time.Minute).UnixNano()})
	if err != nil {
		t.Fatal(err)
	}
	result, err := contract.SealInspectResultV3(contract.InspectResultV3{RequestDigest: inspect.RequestDigest, RequestNotAfterUnixNano: inspect.RequestedNotAfterUnixNano, StartClaim: claim, Journal: journal, HasCleanupClosure: true, CleanupClosure: closure, Phase: contract.HostInspectStartingV3, CheckedUnixNano: now.UnixNano(), ExpiresUnixNano: inspect.RequestedNotAfterUnixNano})
	if err != nil {
		t.Fatal(err)
	}
	if err = result.ValidateFor(inspect, now); err != nil {
		t.Fatal(err)
	}
	bad := result
	bad.HasCleanupClosure = false
	bad.ResultDigest = ""
	if _, err = contract.SealInspectResultV3(bad); !contract.HasCode(err, contract.ErrorConflict) {
		t.Fatalf("presence splice=%v", err)
	}
	stop, err := contract.SealStopRequestV3(contract.StopRequestV3{HostID: claim.HostID, StartID: claim.StartID, StartClaim: claimRef, CleanupClosure: closure, RequestedAtUnixNano: now.UnixNano(), RequestedNotAfterUnixNano: now.Add(time.Hour).UnixNano()})
	if err != nil {
		t.Fatal(err)
	}
	if err = stop.ValidateCurrent(now); err != nil {
		t.Fatal(err)
	}
	spliced := stop
	spliced.CleanupClosure = exactContractV3(t, "praxis.agent-host/cleanup-closure", "closure-2")
	spliced.RequestDigest = ""
	spliced, _ = contract.SealStopRequestV3(spliced)
	if spliced.RequestDigest == stop.RequestDigest {
		t.Fatal("closure splice did not change Stop digest")
	}
}

func TestHostV3StartResultRequiresExactMinimumOwnerWindow(t *testing.T) {
	now := time.Unix(2_400_000_000, 0)
	request := startRequestContractFixtureV3(t, now)
	input, _ := request.ClaimInputV3()
	claim, _ := input.ClaimV1()
	claimRef, _ := claim.CurrentRefV1()
	expires := now.Add(30 * time.Minute).UnixNano()
	ready := contract.SystemReadyCurrentRefV2{ID: "ready-1", Revision: 1, Epoch: 1, Digest: core.DigestBytes([]byte("ready")), ExpiresUnixNano: expires}
	availability := runtimeports.AgentExecutionAvailabilityRefV1{Owner: core.OwnerRef{Domain: "praxis.agent-host", ID: "availability-owner"}, ID: "availability-1", Revision: 1, Epoch: 1, Digest: core.DigestBytes([]byte("availability")), ExpiresUnixNano: now.Add(40 * time.Minute).UnixNano()}
	result, err := contract.SealStartResultV3(contract.StartResultV3{HostID: claim.HostID, StartID: claim.StartID, RequestDigest: request.RequestDigest, RequestNotAfterUnixNano: request.RequestedNotAfterUnixNano, StartClaim: claimRef, Journal: exactContractV3(t, "praxis.agent-host/journal", "journal-1"), CleanupClosure: exactContractV3(t, "praxis.agent-host/cleanup-closure", "closure-1"), Ready: ready, Availability: availability, CheckedUnixNano: now.UnixNano(), ExpiresUnixNano: expires})
	if err != nil {
		t.Fatal(err)
	}
	if err = result.ValidateFor(request, now); err != nil {
		t.Fatal(err)
	}
	bad := result
	bad.ExpiresUnixNano++
	bad.ResultDigest = ""
	if _, err = contract.SealStartResultV3(bad); !contract.HasCode(err, contract.ErrorConflict) {
		t.Fatalf("non-min expiry=%v", err)
	}
	for _, tc := range []struct {
		name   string
		mutate func(*contract.StartResultV3)
	}{
		{"claim-digest", func(v *contract.StartResultV3) { v.StartClaim.Digest = digestContractV3(t, "other-claim") }},
		{"claim-expiry", func(v *contract.StartResultV3) {
			v.StartClaim.ExpiresUnixNano = now.Add(20 * time.Minute).UnixNano()
			v.ExpiresUnixNano = v.StartClaim.ExpiresUnixNano
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			spliced := result
			tc.mutate(&spliced)
			spliced.ResultDigest = ""
			spliced, err = contract.SealStartResultV3(spliced)
			if err != nil {
				t.Fatalf("splice must be standalone-valid: %v", err)
			}
			if err = spliced.ValidateFor(request, now); !contract.HasCode(err, contract.ErrorConflict) {
				t.Fatalf("exact Claim splice=%v", err)
			}
		})
	}
}

func TestHostV3InspectResultRejectsResealedExactClaimSplice(t *testing.T) {
	now := time.Unix(2_400_000_000, 0)
	start := startRequestContractFixtureV3(t, now)
	input, err := start.ClaimInputV3()
	if err != nil {
		t.Fatal(err)
	}
	claim, err := input.ClaimV1()
	if err != nil {
		t.Fatal(err)
	}
	claimRef, err := claim.CurrentRefV1()
	if err != nil {
		t.Fatal(err)
	}
	request, err := contract.SealInspectRequestV3(contract.InspectRequestV3{HostID: claim.HostID, StartID: claim.StartID, StartClaim: claimRef, RequestedAtUnixNano: now.UnixNano(), RequestedNotAfterUnixNano: now.Add(20 * time.Minute).UnixNano()})
	if err != nil {
		t.Fatal(err)
	}
	base := contract.InspectResultV3{RequestDigest: request.RequestDigest, RequestNotAfterUnixNano: request.RequestedNotAfterUnixNano, StartClaim: claim, Journal: exactContractV3(t, "praxis.agent-host/journal", "journal-1"), Phase: contract.HostInspectStartingV3, CheckedUnixNano: now.UnixNano(), ExpiresUnixNano: request.RequestedNotAfterUnixNano}
	for _, tc := range []struct {
		name   string
		mutate func(*contract.HostStartClaimV1)
	}{
		{"claim-digest", func(v *contract.HostStartClaimV1) { v.ConfigDigest = digestContractV3(t, "other-config") }},
		{"claim-expiry", func(v *contract.HostStartClaimV1) { v.ExpiresUnixNano = v.ExpiresUnixNano - int64(time.Minute) }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			splicedClaim := claim
			tc.mutate(&splicedClaim)
			splicedClaim.Digest = ""
			splicedClaim, err = contract.SealHostStartClaimV1(splicedClaim)
			if err != nil {
				t.Fatal(err)
			}
			spliced := base
			spliced.StartClaim = splicedClaim
			spliced, err = contract.SealInspectResultV3(spliced)
			if err != nil {
				t.Fatalf("splice must be standalone-valid: %v", err)
			}
			if err = spliced.ValidateFor(request, now); !contract.HasCode(err, contract.ErrorConflict) {
				t.Fatalf("exact Claim splice=%v", err)
			}
		})
	}
}

func startRequestContractFixtureV3(t *testing.T, now time.Time) contract.StartRequestV3 {
	t.Helper()
	config := contract.HostConfigV1{ContractVersion: contract.ContractVersionV1, HostID: "host-1", DefinitionSourceRef: "definition-source", StatePlaneBindings: []string{"state-a", "state-b"}, ProviderEndpointRefs: []string{"provider-registry"}, SecretBrokerRef: "secret-broker", CatalogRef: "catalog", ResolutionFactsRef: "resolution", RuntimeServiceRefs: []string{"runtime"}, ListenRef: "listen", DiagnosticsPolicyRef: "diagnostics"}
	request, err := contract.SealStartRequestV3(contract.StartRequestV3{StartID: "start-1", DeploymentCurrentRef: contract.HostDeploymentCurrentRefV1{HostID: "host-1", DeploymentID: "deployment-1", Revision: 1, BootstrapDigest: digestContractV3(t, "bootstrap"), ExpiresUnixNano: now.Add(2 * time.Hour).UnixNano(), Digest: digestContractV3(t, "deployment")}, Config: config, DefinitionSourceCurrent: exactContractV3(t, "praxis.agent-definition/definition", "definition-1"), RequestedAtUnixNano: now.UnixNano(), RequestedNotAfterUnixNano: now.Add(time.Hour).UnixNano()})
	if err != nil {
		t.Fatal(err)
	}
	return request
}

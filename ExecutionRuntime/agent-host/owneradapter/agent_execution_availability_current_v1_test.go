package owneradapter

import (
	"context"
	"reflect"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	assemblercontract "github.com/Proview-China/rax/ExecutionRuntime/agent-assembler/contract"
	buildercontract "github.com/Proview-China/rax/ExecutionRuntime/agent-builder/contract"
	definitioncontract "github.com/Proview-China/rax/ExecutionRuntime/agent-definition/contract"
	"github.com/Proview-China/rax/ExecutionRuntime/agent-host/contract"
	hostports "github.com/Proview-China/rax/ExecutionRuntime/agent-host/ports"
	assemblycompiler "github.com/Proview-China/rax/ExecutionRuntime/harness/assemblycompiler"
	assemblycontract "github.com/Proview-China/rax/ExecutionRuntime/harness/assemblycontract"
	assemblytestkit "github.com/Proview-China/rax/ExecutionRuntime/harness/tests/assembly/testkit"
	runtimeconformance "github.com/Proview-China/rax/ExecutionRuntime/runtime/conformance"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	runtimeports "github.com/Proview-China/rax/ExecutionRuntime/runtime/ports"
)

func TestAgentExecutionAvailabilityCurrentReaderV1ReadyHappy(t *testing.T) {
	fixture := newAvailabilityCurrentFixtureV1(t)
	projection, err := fixture.reader.InspectAgentExecutionAvailabilityCurrentV1(context.Background(), fixture.availability)
	if err != nil {
		t.Fatal(err)
	}
	if projection.State != runtimeports.AgentExecutionAvailabilityReadyV1 || projection.Ref != fixture.availability {
		t.Fatalf("projection=%+v", projection)
	}
	if fixture.hostReads.Load() != 2 || fixture.deploymentV2Reads.Load() != 2 || fixture.factReads.Load() != 2 {
		t.Fatalf("host=%d deployment-v2=%d fact=%d", fixture.hostReads.Load(), fixture.deploymentV2Reads.Load(), fixture.factReads.Load())
	}
	if fixture.maxHostWindow.Load() > int64(time.Second) {
		t.Fatalf("Host proof window=%s", time.Duration(fixture.maxHostWindow.Load()))
	}
}

func TestAgentExecutionAvailabilityCurrentReaderV1RuntimeConformance(t *testing.T) {
	fixture := newAvailabilityCurrentFixtureV1(t)
	report, err := runtimeconformance.CheckAgentExecutionAvailabilityV1(context.Background(), runtimeconformance.AgentExecutionAvailabilityCaseV1{
		Reader: fixture.reader,
		Ref:    fixture.availability,
		Now:    fixture.now,
	})
	if err != nil {
		t.Fatal(err)
	}
	if !report.ExactCurrentObserved || !report.ReadyEpochObserved || report.ProductionClaimEligible {
		t.Fatalf("report=%+v", report)
	}
}

func TestAgentExecutionAvailabilityCurrentReaderV1ExactSpliceMatrix(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*availabilityCurrentFixtureV1)
	}{
		{"host phase", func(f *availabilityCurrentFixtureV1) { f.hostPhase = contract.HostInspectStoppingV3 }},
		{"instance claim", func(f *availabilityCurrentFixtureV1) {
			changed := f.claimInput
			changed.ClaimRef.Digest = contract.DigestV1(availabilityDigestV1("another-claim"))
			f.claimInputOverride = &changed
		}},
		{"deployment v1", func(f *availabilityCurrentFixtureV1) {
			changed := f.deploymentV1
			changed.Ref.Revision++
			f.deploymentV1Override = &changed
		}},
		{"deployment v2", func(f *availabilityCurrentFixtureV1) {
			changed := f.deploymentV2
			changed.Ref.BootstrapDigest = contract.DigestV1(availabilityDigestV1("another-bootstrap"))
			f.deploymentV2Override = &changed
		}},
		{"package selection", func(f *availabilityCurrentFixtureV1) {
			changed := f.deploymentV2
			changed.Ref.PackageSelectionRef.Revision++
			changed.Ref.Digest, changed.ProjectionDigest = "", ""
			changed, _ = contract.SealHostDeploymentCurrentV2(changed)
			f.deploymentV2Override = &changed
		}},
		{"system ready", func(f *availabilityCurrentFixtureV1) {
			changed := f.readyFact
			changed.HostID = "another-host"
			f.readyFactOverride = &changed
		}},
		{"availability", func(f *availabilityCurrentFixtureV1) { f.hostAvailabilityDigestDrift = true }},
		{"cleanup", func(f *availabilityCurrentFixtureV1) { f.hostCleanupDigestDrift = true }},
	}
	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			fixture := newAvailabilityCurrentFixtureV1(t)
			testCase.mutate(fixture)
			if _, err := fixture.reader.InspectAgentExecutionAvailabilityCurrentV1(context.Background(), fixture.availability); err == nil {
				t.Fatal("spliced closure returned availability")
			}
		})
	}
}

func TestAgentExecutionAvailabilityCurrentReaderV1FenceDrainStopUnknownAndExpiry(t *testing.T) {
	for _, phase := range []contract.HostInspectPhaseV3{
		contract.HostInspectStoppingV3,
		contract.HostInspectClosedV3,
		contract.HostInspectIndeterminateV3,
	} {
		t.Run(string(phase), func(t *testing.T) {
			fixture := newAvailabilityCurrentFixtureV1(t)
			fixture.fenceV1(t, phase)
			projection, err := fixture.reader.InspectAgentExecutionAvailabilityCurrentV1(context.Background(), fixture.availability)
			if err != nil {
				t.Fatal(err)
			}
			if projection.State != runtimeports.AgentExecutionAvailabilityFencedV1 {
				t.Fatalf("projection=%+v", projection)
			}
			if err = projection.ValidateCurrent(fixture.availability, fixture.now); err == nil {
				t.Fatal("fenced projection admitted new execution")
			}
		})
	}
	t.Run("ready plus indeterminate fails closed", func(t *testing.T) {
		fixture := newAvailabilityCurrentFixtureV1(t)
		fixture.hostPhase = contract.HostInspectIndeterminateV3
		if _, err := fixture.reader.InspectAgentExecutionAvailabilityCurrentV1(context.Background(), fixture.availability); err == nil {
			t.Fatal("indeterminate Host returned ready")
		}
	})
	t.Run("now equals expiry", func(t *testing.T) {
		fixture := newAvailabilityCurrentFixtureV1(t)
		fixture.setClockV1(fixture.expires)
		if _, err := fixture.reader.InspectAgentExecutionAvailabilityCurrentV1(context.Background(), fixture.availability); err == nil {
			t.Fatal("expired availability returned ready")
		}
	})
}

func TestAgentExecutionAvailabilityCurrentReaderV1S1S2ClockNilUnavailableAndCancel(t *testing.T) {
	t.Run("S1 S2 drift", func(t *testing.T) {
		fixture := newAvailabilityCurrentFixtureV1(t)
		fixture.hostJournalDrift = true
		if _, err := fixture.reader.InspectAgentExecutionAvailabilityCurrentV1(context.Background(), fixture.availability); !contract.HasCode(err, contract.ErrorConflict) {
			t.Fatalf("drift error=%v", err)
		}
	})
	t.Run("Package Selection S1 S2 drift", func(t *testing.T) {
		fixture := newAvailabilityCurrentFixtureV1(t)
		changed := fixture.packageSelection
		changed.Ref.Revision++
		changed.Ref.Digest, changed.ProjectionDigest = "", ""
		changed, err := buildercontract.SealAgentPackageSelectionCurrentV1(changed)
		if err != nil {
			t.Fatal(err)
		}
		fixture.packageSelectionSequence = []buildercontract.AgentPackageSelectionCurrentV1{
			fixture.packageSelection,
			fixture.packageSelection,
			changed,
			changed,
		}
		if _, err = fixture.reader.InspectAgentExecutionAvailabilityCurrentV1(context.Background(), fixture.availability); err == nil {
			t.Fatal("Package Selection drift returned availability")
		}
	})
	t.Run("clock rollback", func(t *testing.T) {
		fixture := newAvailabilityCurrentFixtureV1(t)
		fixture.setClockSequenceV1(fixture.now, fixture.now.Add(-time.Nanosecond))
		if _, err := fixture.reader.InspectAgentExecutionAvailabilityCurrentV1(context.Background(), fixture.availability); !contract.HasCode(err, contract.ErrorPrecondition) {
			t.Fatalf("clock error=%v", err)
		}
	})
	t.Run("typed nil", func(t *testing.T) {
		fixture := newAvailabilityCurrentFixtureV1(t)
		var host *availabilityCurrentFixtureV1
		config := fixture.configV1()
		config.Host = host
		if _, err := NewAgentExecutionAvailabilityCurrentReaderV1(config); !contract.HasCode(err, contract.ErrorInvalidArgument) {
			t.Fatalf("typed nil error=%v", err)
		}
	})
	t.Run("unavailable", func(t *testing.T) {
		fixture := newAvailabilityCurrentFixtureV1(t)
		fixture.unavailable = true
		if _, err := fixture.reader.InspectAgentExecutionAvailabilityCurrentV1(context.Background(), fixture.availability); !contract.HasCode(err, contract.ErrorUnavailable) {
			t.Fatalf("unavailable error=%v", err)
		}
	})
	t.Run("cancel", func(t *testing.T) {
		fixture := newAvailabilityCurrentFixtureV1(t)
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		if _, err := fixture.reader.InspectAgentExecutionAvailabilityCurrentV1(ctx, fixture.availability); !contract.HasCode(err, contract.ErrorUnavailable) {
			t.Fatalf("cancel error=%v", err)
		}
	})
}

func TestAgentExecutionAvailabilityCurrentReaderV1RejectsBindingSelectionAndClosureSplice(t *testing.T) {
	t.Run("binding closure digest", func(t *testing.T) {
		fixture := newAvailabilityCurrentFixtureV1(t)
		changed := fixture.packageBinding
		changed.VerifiedPackageClosureDigest = contract.DigestV1(availabilityDigestV1("another-closure"))
		changed.Ref.Digest, changed.BindingDigest = "", ""
		changed, err := contract.SealHostStartPackageSelectionBindingV1(changed)
		if err != nil {
			t.Fatal(err)
		}
		fixture.packageBindingOverride = &changed
		if _, err = fixture.reader.InspectAgentExecutionAvailabilityCurrentV1(context.Background(), fixture.availability); err == nil {
			t.Fatal("binding-to-closure splice returned availability")
		}
	})
	t.Run("another legal Selection current", func(t *testing.T) {
		fixture := newAvailabilityCurrentFixtureV1(t)
		changed := fixture.packageSelection
		changed.Ref.Revision++
		changed.Ref.Digest, changed.ProjectionDigest = "", ""
		changed, err := buildercontract.SealAgentPackageSelectionCurrentV1(changed)
		if err != nil {
			t.Fatal(err)
		}
		fixture.packageSelectionOverride = &changed
		if _, err = fixture.reader.InspectAgentExecutionAvailabilityCurrentV1(context.Background(), fixture.availability); err == nil {
			t.Fatal("another legal Selection returned availability")
		}
	})
	t.Run("another verified closure", func(t *testing.T) {
		fixture := newAvailabilityCurrentFixtureV1(t)
		changed := availabilityVerifiedPackageClosureV1(t)
		changed.Package.PackageID += "-other"
		changed.Package.Digest = ""
		// The body is intentionally no longer the exact verified closure; the
		// public reader must fail closed before returning availability.
		fixture.packageClosureOverride = &changed
		if _, err := fixture.reader.InspectAgentExecutionAvailabilityCurrentV1(context.Background(), fixture.availability); err == nil {
			t.Fatal("spliced verified closure returned availability")
		}
	})
}

func TestAgentExecutionAvailabilityCurrentReaderV1Supports64ConcurrentReads(t *testing.T) {
	fixture := newAvailabilityCurrentFixtureV1(t)
	var successes atomic.Int64
	var wait sync.WaitGroup
	for range 64 {
		wait.Add(1)
		go func() {
			defer wait.Done()
			projection, err := fixture.reader.InspectAgentExecutionAvailabilityCurrentV1(context.Background(), fixture.availability)
			if err != nil {
				t.Errorf("Inspect: %v", err)
				return
			}
			if projection.Ref != fixture.availability {
				t.Errorf("Ref=%+v", projection.Ref)
				return
			}
			successes.Add(1)
		}()
	}
	wait.Wait()
	if successes.Load() != 64 {
		t.Fatalf("successes=%d", successes.Load())
	}
}

func TestAgentExecutionAvailabilityCurrentReaderV1DependenciesAreReadOnly(t *testing.T) {
	configType := reflect.TypeOf(AgentExecutionAvailabilityCurrentReaderConfigV1{})
	if _, exists := configType.FieldByName("DeploymentV2Ref"); exists {
		t.Fatal("caller-supplied DeploymentV2Ref remains in Availability config")
	}
	for index := 0; index < configType.NumField(); index++ {
		field := configType.Field(index)
		if field.Name == "Owner" || field.Name == "Availability" || field.Name == "Clock" {
			continue
		}
		if field.Type.Kind() != reflect.Interface {
			t.Fatalf("%s is not a narrow interface", field.Name)
		}
		for methodIndex := 0; methodIndex < field.Type.NumMethod(); methodIndex++ {
			method := field.Type.Method(methodIndex).Name
			if !strings.HasPrefix(method, "Inspect") && !strings.HasPrefix(method, "ReaderFor") && !strings.HasPrefix(method, "LoadVerified") {
				t.Fatalf("%s exposes write-like method %s", field.Name, method)
			}
		}
	}
}

type availabilityCurrentFixtureV1 struct {
	t                           *testing.T
	now                         time.Time
	expires                     time.Time
	owner                       core.OwnerRef
	availability                runtimeports.AgentExecutionAvailabilityRefV1
	claim                       contract.HostStartClaimV1
	claimInput                  contract.HostStartClaimInputBindingV3
	packageBinding              contract.HostStartPackageSelectionBindingV1
	deploymentV1                contract.HostDeploymentCurrentV1
	deploymentV2                contract.HostDeploymentCurrentV2
	packageSelection            buildercontract.AgentPackageSelectionCurrentV1
	packageClosure              buildercontract.VerifiedAgentPackageClosureV1
	readyFact                   contract.SystemReadyFactV2
	readyCurrent                contract.SystemReadyCurrentV2
	policy                      contract.SystemReadySupervisionPolicyCurrentV2
	cleanup                     contract.HostCleanupClosureFactV2
	reader                      *AgentExecutionAvailabilityCurrentReaderV1
	hostPhase                   contract.HostInspectPhaseV3
	unavailable                 bool
	hostJournalDrift            bool
	hostAvailabilityDigestDrift bool
	hostCleanupDigestDrift      bool
	claimInputOverride          *contract.HostStartClaimInputBindingV3
	packageBindingOverride      *contract.HostStartPackageSelectionBindingV1
	deploymentV1Override        *contract.HostDeploymentCurrentV1
	deploymentV2Override        *contract.HostDeploymentCurrentV2
	packageSelectionOverride    *buildercontract.AgentPackageSelectionCurrentV1
	packageClosureOverride      *buildercontract.VerifiedAgentPackageClosureV1
	packageSelectionSequence    []buildercontract.AgentPackageSelectionCurrentV1
	readyFactOverride           *contract.SystemReadyFactV2
	hostReads                   atomic.Int64
	deploymentV2Reads           atomic.Int64
	factReads                   atomic.Int64
	maxHostWindow               atomic.Int64
	clockMu                     sync.Mutex
	packageMu                   sync.Mutex
	clocks                      []time.Time
}

func newAvailabilityCurrentFixtureV1(t *testing.T) *availabilityCurrentFixtureV1 {
	t.Helper()
	fixture := &availabilityCurrentFixtureV1{
		t:         t,
		now:       time.Unix(1_900_000_000, 0),
		owner:     core.OwnerRef{Domain: "praxis.agent-host", ID: "availability-owner"},
		hostPhase: contract.HostInspectReadyV3,
	}
	fixture.expires = fixture.now.Add(800 * time.Millisecond)
	checked := fixture.now.Add(-100 * time.Millisecond)

	resource := runtimeports.ResourceHandleRefV1{
		Owner:           core.OwnerRef{Domain: "fixture.resources", ID: "resource-owner"},
		ID:              "resource-host",
		Revision:        1,
		Digest:          availabilityDigestV1("resource"),
		Kind:            "fixture/resource",
		ScopeDigest:     availabilityDigestV1("resource-scope"),
		ExpiresUnixNano: fixture.expires.UnixNano(),
	}
	services := availabilityServiceBindingsV1(fixture.expires.UnixNano())
	fixture.packageClosure = availabilityVerifiedPackageClosureV1(t)
	selection, err := buildercontract.SealAgentPackageSelectionCurrentV1(buildercontract.AgentPackageSelectionCurrentV1{
		Ref: buildercontract.AgentPackageSelectionCurrentRefV1{
			SelectionID: "selection/availability", Revision: 1, ExpiresUnixNano: fixture.expires.UnixNano(),
		},
		PackageRef: fixture.packageClosure.Package.RefV1(), PublicationRef: fixture.packageClosure.PublicationRefV2(),
		ClosureDigest:   fixture.packageClosure.ClosureDigest,
		CheckedUnixNano: checked.UnixNano(), ExpiresUnixNano: fixture.expires.UnixNano(),
	})
	if err != nil {
		t.Fatal(err)
	}
	fixture.packageSelection = selection
	deploymentV1, err := contract.SealHostDeploymentCurrentV1(contract.HostDeploymentCurrentV1{
		Ref: contract.HostDeploymentCurrentRefV1{
			HostID: "host-availability", DeploymentID: "deployment-availability", Revision: 1,
			BootstrapDigest: contract.DigestV1(availabilityDigestV1("bootstrap")),
			ExpiresUnixNano: fixture.expires.UnixNano(),
		},
		ResourceHandles: []runtimeports.ResourceHandleRefV1{resource},
		ServiceBindings: services,
		CheckedUnixNano: checked.UnixNano(),
		ExpiresUnixNano: fixture.expires.UnixNano(),
	})
	if err != nil {
		t.Fatal(err)
	}
	fixture.deploymentV1 = deploymentV1
	deploymentV2, err := contract.SealHostDeploymentCurrentV2(contract.HostDeploymentCurrentV2{
		Ref: contract.HostDeploymentCurrentRefV2{
			HostID: "host-availability", DeploymentID: "deployment-availability", Revision: 1,
			BootstrapDigest:     contract.DigestV1(availabilityDigestV1("bootstrap")),
			PackageSelectionRef: fixture.packageSelection.Ref,
			ExpiresUnixNano:     fixture.expires.UnixNano(),
		},
		ResourceHandles: resourcesCloneV1([]runtimeports.ResourceHandleRefV1{resource}),
		ServiceBindings: append([]contract.HostServiceBindingRefV1(nil), services...),
		CheckedUnixNano: checked.UnixNano(),
		ExpiresUnixNano: fixture.expires.UnixNano(),
	})
	if err != nil {
		t.Fatal(err)
	}
	fixture.deploymentV2 = deploymentV2
	input, err := contract.SealHostStartClaimInputV3(contract.HostStartClaimInputV3{
		HostID: "host-availability", StartID: "start-availability",
		DeploymentCurrentRef: deploymentV1.Ref,
		HostConfigDigest:     contract.DigestV1(availabilityDigestV1("host-config")),
		DefinitionSourceRef:  availabilityExactRefV1("praxis.agent-definition/source-current", "definition-source"),
		RequestedOperation:   contract.HostStartOperationStartV1,
		CreatedUnixNano:      fixture.now.Add(-time.Second).UnixNano(),
		ExpiresUnixNano:      fixture.expires.UnixNano(),
	})
	if err != nil {
		t.Fatal(err)
	}
	claim, err := input.ClaimV1()
	if err != nil {
		t.Fatal(err)
	}
	fixture.claim = claim
	fixture.claimInput, err = contract.NewHostStartClaimInputBindingV3(claim, input)
	if err != nil {
		t.Fatal(err)
	}
	fixture.packageBinding, err = contract.NewHostStartPackageSelectionBindingV1(
		claim,
		fixture.claimInput,
		fixture.deploymentV2,
		fixture.packageSelection,
		fixture.now.Add(-50*time.Millisecond).UnixNano(),
	)
	if err != nil {
		t.Fatal(err)
	}
	claimRef, _ := claim.CurrentRefV1()
	component := availabilityComponentV1(fixture.expires.UnixNano())
	refs := make([]runtimeports.OwnerCurrentRefV1, 11)
	for index := range refs {
		refs[index] = availabilityOwnerCurrentV1("system-ready-"+string(rune('a'+index)), fixture.expires.UnixNano())
	}
	minimumWindow := int64(100 * time.Millisecond)
	fact, err := contract.SealSystemReadyFactV2(contract.SystemReadyFactV2{
		Ref:    contract.SystemReadyFactRefV2{ExpiresUnixNano: fixture.expires.UnixNano()},
		HostID: "host-availability", StartID: "start-availability", HostStartClaim: claimRef,
		DefinitionCurrent: refs[0], PlanCurrent: refs[1], AssemblyCurrent: refs[2], BindingSetCurrent: refs[3],
		ActivationCurrent: refs[4], GenerationBindingCurrent: refs[5], ApplicationStartCurrent: refs[6],
		SandboxLeaseCurrent: refs[7], SandboxActiveCurrent: refs[8], ExecutionReadyCurrent: refs[9],
		SupervisionPolicyCurrent: refs[10], Components: []contract.ComponentProductionCurrentV2{component},
		MinimumReadyWindowNanos: minimumWindow,
		CheckedUnixNano:         checked.UnixNano(), ExpiresUnixNano: fixture.expires.UnixNano(),
	})
	if err != nil {
		t.Fatal(err)
	}
	fixture.readyFact = fact
	current, err := contract.SealSystemReadyCurrentV2(contract.SystemReadyCurrentV2{
		Ref:    contract.SystemReadyCurrentRefV2{ID: contract.DeriveSystemReadyCurrentIDV2("host-availability", "start-availability"), Revision: 1, Epoch: 7},
		HostID: "host-availability", StartID: "start-availability", FactRef: fact.Ref,
		State: contract.SystemReadyCurrentReadyV2, CheckedUnixNano: checked.UnixNano(), ExpiresUnixNano: fixture.expires.UnixNano(),
	})
	if err != nil {
		t.Fatal(err)
	}
	fixture.readyCurrent = current
	projection, err := current.ToAgentExecutionAvailabilityV1(fixture.owner)
	if err != nil {
		t.Fatal(err)
	}
	fixture.availability = projection.Ref
	fixture.policy, err = contract.SealSystemReadySupervisionPolicyCurrentV2(contract.SystemReadySupervisionPolicyCurrentV2{
		Ref: refs[10], MinimumReadyWindowNanos: minimumWindow,
		CheckedUnixNano: checked.UnixNano(), ExpiresUnixNano: fixture.expires.UnixNano(),
	})
	if err != nil {
		t.Fatal(err)
	}
	fixture.cleanup = availabilityCleanupClosureV1(t, claim, checked, fixture.expires)
	fixture.rebuildReaderV1()
	return fixture
}

func (fixture *availabilityCurrentFixtureV1) configV1() AgentExecutionAvailabilityCurrentReaderConfigV1 {
	return AgentExecutionAvailabilityCurrentReaderConfigV1{
		Owner: fixture.owner, Availability: fixture.availability,
		Host: fixture, Claims: fixture, ClaimInputs: fixture, StartPackageSelectionBindings: fixture,
		DeploymentsV1: fixture, DeploymentsV2: fixture, PackageSelections: fixture,
		SystemReadyCurrent: fixture, SystemReadyFacts: fixture, SystemReadyCore: fixture,
		ComponentCurrents: fixture, CleanupClosures: fixture, Clock: fixture.clockV1,
	}
}

func (fixture *availabilityCurrentFixtureV1) rebuildReaderV1() {
	fixture.t.Helper()
	reader, err := NewAgentExecutionAvailabilityCurrentReaderV1(fixture.configV1())
	if err != nil {
		fixture.t.Fatal(err)
	}
	fixture.reader = reader
}

func (fixture *availabilityCurrentFixtureV1) setClockV1(now time.Time) {
	fixture.setClockSequenceV1(now)
}

func (fixture *availabilityCurrentFixtureV1) setClockSequenceV1(values ...time.Time) {
	fixture.clockMu.Lock()
	fixture.clocks = append([]time.Time(nil), values...)
	fixture.clockMu.Unlock()
	fixture.rebuildReaderV1()
}

func (fixture *availabilityCurrentFixtureV1) clockV1() time.Time {
	fixture.clockMu.Lock()
	defer fixture.clockMu.Unlock()
	if len(fixture.clocks) == 0 {
		return fixture.now
	}
	value := fixture.clocks[0]
	fixture.clocks = fixture.clocks[1:]
	return value
}

func (fixture *availabilityCurrentFixtureV1) fenceV1(t *testing.T, phase contract.HostInspectPhaseV3) {
	t.Helper()
	fenced := fixture.readyCurrent
	fenced.Ref.Revision++
	fenced.State = contract.SystemReadyCurrentFencedV2
	fenced.Ref.Digest, fenced.ProjectionDigest = "", ""
	fenced, err := contract.SealSystemReadyCurrentV2(fenced)
	if err != nil {
		t.Fatal(err)
	}
	fixture.readyCurrent = fenced
	projection, err := fenced.ToAgentExecutionAvailabilityV1(fixture.owner)
	if err != nil {
		t.Fatal(err)
	}
	fixture.availability = projection.Ref
	fixture.hostPhase = phase
	fixture.rebuildReaderV1()
}

func (fixture *availabilityCurrentFixtureV1) InspectV3(_ context.Context, request contract.InspectRequestV3) (contract.InspectResultV3, error) {
	if fixture.unavailable {
		return contract.InspectResultV3{}, contract.NewError(contract.ErrorUnavailable, "fixture_unavailable", "fixture unavailable")
	}
	call := fixture.hostReads.Add(1)
	window := request.RequestedNotAfterUnixNano - request.RequestedAtUnixNano
	for {
		current := fixture.maxHostWindow.Load()
		if current >= window || fixture.maxHostWindow.CompareAndSwap(current, window) {
			break
		}
	}
	journal := availabilityExactRefV1("praxis.agent-host/journal-v2", "journal")
	if fixture.hostJournalDrift && call > 1 {
		journal = availabilityExactRefV1("praxis.agent-host/journal-v2", "journal-s2")
	}
	availability := fixture.availability
	if fixture.hostAvailabilityDigestDrift {
		availability.Digest = availabilityDigestV1("availability-splice")
	}
	cleanupRef, _ := fixture.cleanup.RefV2()
	cleanup := contract.ExactRefV1{Kind: contract.HostCleanupClosureRefKindV2, ID: cleanupRef.ClosureID, Revision: cleanupRef.Revision, Digest: cleanupRef.Digest}
	if fixture.hostCleanupDigestDrift {
		cleanup.Digest = contract.DigestV1(availabilityDigestV1("cleanup-splice"))
	}
	return contract.SealInspectResultV3(contract.InspectResultV3{
		RequestDigest: request.RequestDigest, RequestNotAfterUnixNano: request.RequestedNotAfterUnixNano,
		StartClaim: fixture.claim, Journal: journal,
		HasReady: true, Ready: fixture.readyCurrent.Ref,
		HasAvailability: true, Availability: availability,
		HasCleanupClosure: true, CleanupClosure: cleanup,
		Phase: fixture.hostPhase, CheckedUnixNano: request.RequestedAtUnixNano,
		ExpiresUnixNano: request.RequestedNotAfterUnixNano,
	})
}

func (fixture *availabilityCurrentFixtureV1) InspectHostStartClaimCurrentV1(context.Context, contract.HostStartClaimRefV1) (contract.HostStartClaimV1, error) {
	return fixture.claim, nil
}

func (fixture *availabilityCurrentFixtureV1) InspectHostStartClaimInputV3(context.Context, contract.HostStartClaimRefV1) (contract.HostStartClaimInputBindingV3, error) {
	if fixture.claimInputOverride != nil {
		return *fixture.claimInputOverride, nil
	}
	return fixture.claimInput, nil
}

func (fixture *availabilityCurrentFixtureV1) InspectHostStartPackageSelectionBindingForClaimV1(
	context.Context,
	contract.HostStartClaimRefV1,
) (contract.HostStartPackageSelectionBindingV1, error) {
	if fixture.packageBindingOverride != nil {
		return *fixture.packageBindingOverride, nil
	}
	return fixture.packageBinding, nil
}

func (fixture *availabilityCurrentFixtureV1) InspectHostDeploymentCurrentV1(context.Context, contract.HostDeploymentCurrentRefV1) (contract.HostDeploymentCurrentV1, error) {
	if fixture.deploymentV1Override != nil {
		return *fixture.deploymentV1Override, nil
	}
	return fixture.deploymentV1, nil
}

func (fixture *availabilityCurrentFixtureV1) InspectHostDeploymentCurrentV2(context.Context, contract.HostDeploymentCurrentRefV2) (contract.HostDeploymentCurrentV2, error) {
	fixture.deploymentV2Reads.Add(1)
	if fixture.deploymentV2Override != nil {
		return *fixture.deploymentV2Override, nil
	}
	return fixture.deploymentV2, nil
}

func (fixture *availabilityCurrentFixtureV1) InspectAgentPackageSelectionExactV1(
	context.Context,
	buildercontract.AgentPackageSelectionCurrentRefV1,
) (buildercontract.AgentPackageSelectionCurrentV1, error) {
	fixture.packageMu.Lock()
	defer fixture.packageMu.Unlock()
	if len(fixture.packageSelectionSequence) != 0 {
		value := fixture.packageSelectionSequence[0]
		fixture.packageSelectionSequence = fixture.packageSelectionSequence[1:]
		return buildercontract.CloneAgentPackageSelectionCurrentV1(value), nil
	}
	if fixture.packageSelectionOverride != nil {
		return buildercontract.CloneAgentPackageSelectionCurrentV1(*fixture.packageSelectionOverride), nil
	}
	return buildercontract.CloneAgentPackageSelectionCurrentV1(fixture.packageSelection), nil
}

func (fixture *availabilityCurrentFixtureV1) InspectAgentPackageSelectionCurrentV1(
	context.Context,
	string,
) (buildercontract.AgentPackageSelectionCurrentV1, error) {
	fixture.packageMu.Lock()
	defer fixture.packageMu.Unlock()
	if len(fixture.packageSelectionSequence) != 0 {
		value := fixture.packageSelectionSequence[0]
		fixture.packageSelectionSequence = fixture.packageSelectionSequence[1:]
		return buildercontract.CloneAgentPackageSelectionCurrentV1(value), nil
	}
	if fixture.packageSelectionOverride != nil {
		return buildercontract.CloneAgentPackageSelectionCurrentV1(*fixture.packageSelectionOverride), nil
	}
	return buildercontract.CloneAgentPackageSelectionCurrentV1(fixture.packageSelection), nil
}

func (fixture *availabilityCurrentFixtureV1) LoadVerifiedAgentPackageClosureV1(
	context.Context,
	buildercontract.AgentPackageRefV1,
) (buildercontract.VerifiedAgentPackageClosureV1, error) {
	if fixture.packageClosureOverride != nil {
		return buildercontract.CloneVerifiedAgentPackageClosureV1(*fixture.packageClosureOverride), nil
	}
	return buildercontract.CloneVerifiedAgentPackageClosureV1(fixture.packageClosure), nil
}

func (fixture *availabilityCurrentFixtureV1) InspectSystemReadyCurrentForAvailabilityV2(context.Context, runtimeports.AgentExecutionAvailabilityRefV1) (contract.SystemReadyCurrentV2, error) {
	return fixture.readyCurrent, nil
}

func (fixture *availabilityCurrentFixtureV1) InspectSystemReadyFactV2(context.Context, contract.SystemReadyFactRefV2) (contract.SystemReadyFactV2, error) {
	fixture.factReads.Add(1)
	if fixture.readyFactOverride != nil {
		return *fixture.readyFactOverride, nil
	}
	return fixture.readyFact, nil
}

func (fixture *availabilityCurrentFixtureV1) InspectDefinitionCurrentV2(_ context.Context, ref runtimeports.OwnerCurrentRefV1) (runtimeports.OwnerCurrentRefV1, error) {
	return ref, nil
}
func (fixture *availabilityCurrentFixtureV1) InspectPlanCurrentV2(_ context.Context, ref runtimeports.OwnerCurrentRefV1) (runtimeports.OwnerCurrentRefV1, error) {
	return ref, nil
}
func (fixture *availabilityCurrentFixtureV1) InspectAssemblyCurrentV2(_ context.Context, ref runtimeports.OwnerCurrentRefV1) (runtimeports.OwnerCurrentRefV1, error) {
	return ref, nil
}
func (fixture *availabilityCurrentFixtureV1) InspectBindingSetCurrentV2(_ context.Context, ref runtimeports.OwnerCurrentRefV1) (runtimeports.OwnerCurrentRefV1, error) {
	return ref, nil
}
func (fixture *availabilityCurrentFixtureV1) InspectActivationCurrentV2(_ context.Context, ref runtimeports.OwnerCurrentRefV1) (runtimeports.OwnerCurrentRefV1, error) {
	return ref, nil
}
func (fixture *availabilityCurrentFixtureV1) InspectGenerationBindingCurrentV2(_ context.Context, ref runtimeports.OwnerCurrentRefV1) (runtimeports.OwnerCurrentRefV1, error) {
	return ref, nil
}
func (fixture *availabilityCurrentFixtureV1) InspectApplicationStartCurrentV2(_ context.Context, ref runtimeports.OwnerCurrentRefV1) (runtimeports.OwnerCurrentRefV1, error) {
	return ref, nil
}
func (fixture *availabilityCurrentFixtureV1) InspectSandboxLeaseCurrentV2(_ context.Context, ref runtimeports.OwnerCurrentRefV1) (runtimeports.OwnerCurrentRefV1, error) {
	return ref, nil
}
func (fixture *availabilityCurrentFixtureV1) InspectSandboxActiveCurrentV2(_ context.Context, ref runtimeports.OwnerCurrentRefV1) (runtimeports.OwnerCurrentRefV1, error) {
	return ref, nil
}
func (fixture *availabilityCurrentFixtureV1) InspectExecutionReadyCurrentV2(_ context.Context, ref runtimeports.OwnerCurrentRefV1) (runtimeports.OwnerCurrentRefV1, error) {
	return ref, nil
}
func (fixture *availabilityCurrentFixtureV1) InspectSupervisionPolicyCurrentV2(context.Context, runtimeports.OwnerCurrentRefV1) (contract.SystemReadySupervisionPolicyCurrentV2, error) {
	return fixture.policy, nil
}
func (fixture *availabilityCurrentFixtureV1) ReaderForComponentProductionCurrentV2(runtimeports.NamespacedNameV2) (hostports.ComponentProductionCurrentReaderV2, error) {
	return fixture, nil
}
func (fixture *availabilityCurrentFixtureV1) InspectComponentProductionCurrentV2(_ context.Context, expected contract.ComponentProductionCurrentV2) (contract.ComponentProductionCurrentV2, error) {
	return expected, nil
}
func (fixture *availabilityCurrentFixtureV1) InspectHostCleanupClosureForStartV2(context.Context, string, string) (contract.HostCleanupClosureFactV2, error) {
	return fixture.cleanup, nil
}

func availabilityServiceBindingsV1(expires int64) []contract.HostServiceBindingRefV1 {
	roles := []contract.HostServiceBindingRoleV1{
		contract.HostServiceDefinitionSourceV1, contract.HostServiceCatalogV1, contract.HostServiceResolutionFactsV1,
		contract.HostServiceSecretBrokerV1, contract.HostServiceCredentialRegistryV1, contract.HostServiceProviderRegistryV1,
		contract.HostServiceRuntimeV1, contract.HostServiceApplicationV1, contract.HostServiceHarnessV1,
		contract.HostServiceListenV1, contract.HostServiceDiagnosticsV1, contract.HostServiceShutdownV1,
	}
	result := make([]contract.HostServiceBindingRefV1, 0, len(roles))
	for index, role := range roles {
		id := "service-" + string(rune('a'+index))
		result = append(result, contract.HostServiceBindingRefV1{
			Role: role, ConfiguredID: id, BindingRef: availabilityExactRefV1("fixture/service-binding", id),
			Capability: "fixture/service", ExpiresUnixNano: expires,
		})
	}
	return result
}

func availabilityComponentV1(expires int64) contract.ComponentProductionCurrentV2 {
	domain := runtimeports.NamespacedNameV2("fixture/component")
	return contract.ComponentProductionCurrentV2{
		Domain:               domain,
		ReleaseCurrent:       availabilityOwnerCurrentV1("component-release", expires),
		ConstructedComponent: availabilityExactRefV1("fixture/component-instance", "component-instance"),
		Binding: runtimeports.BindingAdmissionBindingRefV1{
			ComponentID: runtimeports.ComponentIDV2(domain), ID: "component-binding", Revision: 1,
			Digest: availabilityDigestV1("component-binding"), ExpiresUnixNano: expires,
		},
		GenerationCurrent: availabilityOwnerCurrentV1("component-generation", expires),
		ActivationCurrent: availabilityOwnerCurrentV1("component-activation", expires),
		ProductionCurrent: availabilityOwnerCurrentV1("component-production", expires),
	}
}

func availabilityOwnerCurrentV1(id string, expires int64) runtimeports.OwnerCurrentRefV1 {
	return runtimeports.OwnerCurrentRefV1{
		Owner:           core.OwnerRef{Domain: "fixture.owner", ID: core.OwnerID("owner-" + id)},
		ContractVersion: "1.0.0", ID: id, Revision: 1,
		Digest: availabilityDigestV1("owner-current-" + id), ExpiresUnixNano: expires,
	}
}

func availabilityExactRefV1(kind, id string) contract.ExactRefV1 {
	return contract.ExactRefV1{Kind: kind, ID: id, Revision: 1, Digest: contract.DigestV1(availabilityDigestV1(kind + ":" + id))}
}

func availabilityDigestV1(value string) core.Digest {
	return core.DigestBytes([]byte(value))
}

func availabilityVerifiedPackageClosureV1(t *testing.T) buildercontract.VerifiedAgentPackageClosureV1 {
	t.Helper()
	input := assemblytestkit.ValidInput()
	compiled, err := assemblycompiler.New().Compile(input)
	if err != nil {
		t.Fatal(err)
	}
	publication, err := assemblycontract.NewAssemblyPublicationBundleV2(input.ScopeRef, compiled)
	if err != nil {
		t.Fatal(err)
	}
	publicationRef := assemblycontract.AssemblyPublicationRefV2{
		PublicationID: publication.Publication.PublicationID,
		Revision:      publication.Publication.Revision,
		Digest:        publication.Publication.Digest,
	}
	lock, err := buildercontract.SealLockManifestV1(buildercontract.AgentPackageLockManifestV1{
		DefinitionRef:      definitioncontract.AgentDefinitionRefV1{DefinitionID: "agent/availability", Revision: 1, Digest: availabilityDigestV1("definition")},
		ResolvedPlanRef:    assemblercontract.ResolvedAgentPlanRefV1{PlanID: "plan/availability", Revision: 1, Digest: availabilityDigestV1("plan")},
		ResolutionFactsRef: assemblercontract.ResolutionFactsRefV1{FactsID: "facts/availability", Revision: 1, Digest: availabilityDigestV1("facts")},
		CatalogRef:         assemblercontract.ComponentReleaseCatalogRefV1{CatalogID: "catalog/availability", Revision: 1, Digest: availabilityDigestV1("catalog")},
		ComponentReleaseRefs: []assemblercontract.ComponentReleaseRefV1{{
			ReleaseID: "release/availability", Revision: 1, Digest: availabilityDigestV1("release"), ComponentID: "praxis/availability",
		}},
		BindingPlanDigest:      availabilityDigestV1("binding-plan"),
		AssemblyInputDigest:    input.Digest,
		FrozenUnixNano:         input.CreatedUnixNano,
		HarnessCompilerVersion: assemblycontract.CompilerVersionV1,
		PublicationRef:         publicationRef,
		GenerationRef:          publication.Publication.Artifacts.Generation,
		ManifestRef:            publication.Publication.Artifacts.Manifest,
		GraphRef:               publication.Publication.Artifacts.Graph,
		HandoffRef:             publication.Publication.Artifacts.Handoff,
	})
	if err != nil {
		t.Fatal(err)
	}
	pkg, err := buildercontract.SealPackageV1(buildercontract.AgentPackageV1{Lock: lock})
	if err != nil {
		t.Fatal(err)
	}
	closure, err := buildercontract.SealVerifiedAgentPackageClosureV1(pkg, publication)
	if err != nil {
		t.Fatal(err)
	}
	return closure
}

func resourcesCloneV1(values []runtimeports.ResourceHandleRefV1) []runtimeports.ResourceHandleRefV1 {
	return append([]runtimeports.ResourceHandleRefV1(nil), values...)
}

func availabilityCleanupClosureV1(t *testing.T, claim contract.HostStartClaimV1, checked, expires time.Time) contract.HostCleanupClosureFactV2 {
	t.Helper()
	claimRef, err := claim.CurrentRefV1()
	if err != nil {
		t.Fatal(err)
	}
	ownerCurrent := func(id string) runtimeports.OwnerCurrentRefV1 {
		return availabilityOwnerCurrentV1("cleanup-"+id, expires.UnixNano())
	}
	component := runtimeports.ComponentIDV2("fixture/control")
	binding := runtimeports.BindingAdmissionBindingRefV1{ComponentID: component, ID: "binding-control", Revision: 1, Digest: availabilityDigestV1("cleanup-binding"), ExpiresUnixNano: expires.UnixNano()}
	resourceSet := runtimeports.ResourceBindingSetRefV1{ID: "resource-set", Revision: 1, Digest: availabilityDigestV1("resource-set"), ExpiresUnixNano: expires.UnixNano()}
	handle := runtimeports.ResourceHandleRefV1{Owner: core.OwnerRef{Domain: "fixture.resources", ID: "owner-resource"}, ID: "resource-control", Revision: 1, Digest: availabilityDigestV1("cleanup-resource"), Kind: "fixture/resource", ScopeDigest: availabilityDigestV1("cleanup-scope"), ExpiresUnixNano: expires.UnixNano()}
	generation := ownerCurrent("generation")
	factory := contract.ControlAdapterFactoryRefV2{FactoryID: "factory/control", Revision: 1, Digest: availabilityDigestV1("factory")}
	controlNode := availabilityCleanupNodeV1(t, "control-cleanup", contract.CleanupOwnerNodeV2, string(component), contract.CleanupLiveExecutionV2)
	harness := availabilityCleanupNodeV1(t, contract.CleanupBarrierHarnessCloseV2, contract.CleanupBarrierNodeV2, contract.CleanupBarrierOwnerHarnessV2, contract.CleanupLiveExecutionV2, controlNode.NodeID)
	fence := availabilityCleanupNodeV1(t, contract.CleanupBarrierSandboxFenceV2, contract.CleanupBarrierNodeV2, contract.CleanupBarrierOwnerSandboxV2, contract.CleanupFencedSandboxLeaseV2, harness.NodeID)
	release := availabilityCleanupNodeV1(t, contract.CleanupBarrierSandboxReleaseV2, contract.CleanupBarrierNodeV2, contract.CleanupBarrierOwnerSandboxV2, contract.CleanupFencedSandboxLeaseV2, fence.NodeID)
	aggregate := availabilityCleanupNodeV1(t, contract.CleanupBarrierRuntimeCleanupAggregateV2, contract.CleanupBarrierNodeV2, contract.CleanupBarrierOwnerRuntimeV2, contract.CleanupHostControlHandleV2, release.NodeID)
	plan, err := contract.SealCleanupPlanV2(contract.CleanupPlanV2{ContractVersion: contract.CleanupContractVersionV2, PlanID: "plan/" + claim.StartID, Revision: 1, HostID: claim.HostID, StartID: claim.StartID, Nodes: []contract.CleanupNodeV2{controlNode, harness, fence, release, aggregate}})
	if err != nil {
		t.Fatal(err)
	}
	route := contract.HostCleanupPlanTemplateRouteV2{NodeID: controlNode.NodeID, FactoryRef: factory, ComponentID: component, ArtifactDigest: availabilityDigestV1("artifact"), Capability: "fixture/control-cleanup", Binding: binding, CleanupContractRef: controlNode.CleanupContractRef, InspectPortBinding: controlNode.InspectPortBinding, RequestSchemaDigest: controlNode.RequestSchemaDigest, ResultSchemaDigest: controlNode.ResultSchemaDigest, ResourceClass: controlNode.ResourceClass}
	template, err := contract.SealHostCleanupPlanTemplateCurrentV2(contract.HostCleanupPlanTemplateCurrentV2{TemplateRef: contract.ExactRefV1{Kind: contract.HostCleanupPlanTemplateRefKindV2, ID: "template/" + claim.StartID, Revision: 1}, Routes: []contract.HostCleanupPlanTemplateRouteV2{route}, FixedBarriers: []contract.CleanupNodeV2{harness, fence, release, aggregate}, ResourceBindingSet: resourceSet, CheckedUnixNano: checked.UnixNano(), ExpiresUnixNano: expires.UnixNano()})
	if err != nil {
		t.Fatal(err)
	}
	control := contract.HostCleanupClosureControlCoverageV2{FactoryRef: factory, ComponentID: component, ArtifactDigest: route.ArtifactDigest, Capability: route.Capability, Binding: binding, Generation: generation, ResourceBindingSet: resourceSet, ResourceHandles: []runtimeports.ResourceHandleRefV1{handle}, CleanupNodeIDs: []string{controlNode.NodeID}}
	coverage := []contract.HostCleanupClosureCoverageEntryV2{
		{SourceKind: contract.HostCleanupCoverageBindingV2, SourceID: binding.ID, SourceRevision: uint64(binding.Revision), SourceDigest: contract.DigestV1(binding.Digest), ComponentID: string(component), ResourceClass: controlNode.ResourceClass, CleanupNodeID: controlNode.NodeID},
		{SourceKind: contract.HostCleanupCoverageControlV2, SourceID: factory.FactoryID, SourceRevision: uint64(factory.Revision), SourceDigest: contract.DigestV1(factory.Digest), ComponentID: string(component), ResourceClass: controlNode.ResourceClass, CleanupNodeID: controlNode.NodeID},
		{SourceKind: contract.HostCleanupCoverageResourceV2, SourceID: handle.ID, SourceRevision: uint64(handle.Revision), SourceDigest: contract.DigestV1(handle.Digest), ComponentID: string(component), ResourceClass: controlNode.ResourceClass, CleanupNodeID: controlNode.NodeID},
	}
	for _, node := range []contract.CleanupNodeV2{harness, fence, release, aggregate} {
		coverage = append(coverage, contract.HostCleanupClosureCoverageEntryV2{SourceKind: contract.HostCleanupCoverageBarrierV2, SourceID: node.NodeID, SourceRevision: 1, SourceDigest: node.Digest, ComponentID: node.OwnerComponentID, ResourceClass: node.ResourceClass, CleanupNodeID: node.NodeID})
	}
	assembly := contract.HostCleanupClosureAssemblyCoordinateV2{ScopeRef: "scope/" + claim.StartID, AssemblyInput: availabilityExactRefV1("praxis.harness/assembly-input", "input"), Publication: availabilityExactRefV1("praxis.harness/publication", "publication"), Generation: availabilityExactRefV1("praxis.harness/generation", "generation"), Manifest: availabilityExactRefV1("praxis.harness/manifest", "manifest"), Graph: availabilityExactRefV1("praxis.harness/graph", "graph"), Handoff: availabilityExactRefV1("praxis.harness/handoff", "handoff"), OwnerCurrent: generation}
	fact, err := contract.SealHostCleanupClosureFactV2(contract.HostCleanupClosureFactV2{Revision: 1, StartClaimRef: claimRef, Assembly: assembly, Binding: contract.HostCleanupClosureBindingCoordinateV2{AttemptID: "binding-attempt", RequestDigest: availabilityDigestV1("binding-request"), BindingSet: runtimeports.BindingAdmissionBindingSetRefV1{ID: "binding-set", Revision: 1, Digest: availabilityDigestV1("binding-set"), ExpiresUnixNano: expires.UnixNano()}, Bindings: []runtimeports.BindingAdmissionBindingRefV1{binding}, ResourceBindingSet: resourceSet, CheckedUnixNano: checked.UnixNano(), ExpiresUnixNano: expires.UnixNano(), ResultDigest: availabilityDigestV1("binding-result")}, PlanTemplate: template, Controls: []contract.HostCleanupClosureControlCoverageV2{control}, Plan: plan, Coverage: coverage, CreatedUnixNano: checked.Add(50 * time.Millisecond).UnixNano()})
	if err != nil {
		t.Fatal(err)
	}
	return fact
}

func availabilityCleanupNodeV1(t *testing.T, id string, kind contract.CleanupNodeKindV2, owner string, class contract.CleanupResourceClassV2, dependencies ...string) contract.CleanupNodeV2 {
	t.Helper()
	node, err := contract.SealCleanupNodeV2(contract.CleanupNodeV2{NodeID: id, Kind: kind, OwnerComponentID: owner, CleanupContractRef: availabilityExactRefV1("fixture/cleanup-contract", "cleanup-"+id), ResourceClass: class, RequiredBarrierIDs: dependencies, InspectPortBinding: availabilityExactRefV1("fixture/inspect-port", "inspect-"+id), RequestSchemaDigest: contract.DigestV1(availabilityDigestV1("request-" + id)), ResultSchemaDigest: contract.DigestV1(availabilityDigestV1("result-" + id))})
	if err != nil {
		t.Fatal(err)
	}
	return node
}

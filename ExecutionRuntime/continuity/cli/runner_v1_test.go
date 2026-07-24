package cli_test

import (
	"bytes"
	"context"
	"encoding/json"
	"sync"
	"testing"
	"time"

	appcontract "github.com/Proview-China/rax/ExecutionRuntime/application/contract"
	"github.com/Proview-China/rax/ExecutionRuntime/continuity/cli"
	continuitycontract "github.com/Proview-China/rax/ExecutionRuntime/continuity/contract"
	"github.com/Proview-China/rax/ExecutionRuntime/continuity/domain"
	"github.com/Proview-China/rax/ExecutionRuntime/continuity/internal/testkit"
	continuityports "github.com/Proview-China/rax/ExecutionRuntime/continuity/ports"
	"github.com/Proview-China/rax/ExecutionRuntime/continuity/sdk"
	"github.com/Proview-China/rax/ExecutionRuntime/continuity/storage/memory"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	runtimeports "github.com/Proview-China/rax/ExecutionRuntime/runtime/ports"
)

func TestRunnerV1MapsAllGovernedCommandsAndInspect(t *testing.T) {
	now := time.Unix(1_900_000_000, 0)
	cases := []struct {
		args []string
		kind appcontract.ContinuityWorkflowKindV1
	}{{[]string{"timeline", "project"}, appcontract.ContinuityTimelineProjectV1}, {[]string{"checkpoint", "create"}, appcontract.ContinuityCheckpointCreateV1}, {[]string{"fork"}, appcontract.ContinuityForkV1}, {[]string{"rewind", "plan"}, appcontract.ContinuityRewindPlanV1}, {[]string{"restore"}, appcontract.ContinuityRestoreV1}, {[]string{"artifact", "attach"}, appcontract.ContinuityArtifactAttachV1}, {[]string{"retention", "resolve"}, appcontract.ContinuityRetentionResolveV1}}
	for _, test := range cases {
		port := &cliWorkflowPortV1{}
		runner, err := cli.NewRunnerV1(port)
		if err != nil {
			t.Fatal(err)
		}
		request := cliWorkflowRequestV1(now, test.kind)
		input, _ := json.Marshal(request)
		var output bytes.Buffer
		if err := runner.RunV1(context.Background(), test.args, bytes.NewReader(input), &output); err != nil {
			t.Fatalf("%v: %v", test.args, err)
		}
		var decoded cli.OutputV1
		if err := json.Unmarshal(output.Bytes(), &decoded); err != nil || decoded.ContractVersion != cli.ContractVersionV1 || decoded.Inspection.RequestDigest == "" || port.submitCalls != 1 {
			t.Fatalf("%v output/calls invalid: %#v calls=%d err=%v", test.args, decoded, port.submitCalls, err)
		}
		output.Reset()
		if err := runner.RunV1(context.Background(), []string{"workflow", "inspect"}, bytes.NewReader(input), &output); err != nil || port.inspectCalls != 1 {
			t.Fatalf("workflow inspect = calls=%d err=%v", port.inspectCalls, err)
		}
	}
}

func TestRunnerV1TimelineShowWatchAndCheckpointInspect(t *testing.T) {
	now := time.Date(2026, 7, 18, 10, 30, 0, 0, time.UTC)
	backend := memory.NewWithClock(func() time.Time { return now })
	clock := &testkit.Clock{Time: now}
	timeline, err := domain.NewReferenceTimeline(backend, clock, time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	record, _, err := timeline.Project(context.Background(), testkit.Candidate(1, 1, continuitycontract.TrustObservation))
	if err != nil {
		t.Fatal(err)
	}
	checkpoint, err := domain.NewCheckpointManifestControllerV2(backend)
	if err != nil {
		t.Fatal(err)
	}
	manifest := testkit.ManifestV2(continuitycontract.ManifestCollecting, 1)
	if _, _, err := checkpoint.CreateCheckpointManifestV2(context.Background(), continuityports.CreateCheckpointManifestRequestV2{Candidate: manifest, ExpectAbsent: true}); err != nil {
		t.Fatal(err)
	}
	client, err := sdk.New(sdk.Config{Timeline: timeline, Checkpoints: checkpoint, Clock: func() time.Time { return now }})
	if err != nil {
		t.Fatal(err)
	}
	runner, err := cli.NewRunnerWithReadersV1(&cliWorkflowPortV1{}, client, client)
	if err != nil {
		t.Fatal(err)
	}

	showInput, _ := json.Marshal(cli.TimelineShowRequestV1{EvidenceRecordRef: record.EvidenceRecordRef})
	var showOutput bytes.Buffer
	if err := runner.RunV1(context.Background(), []string{"timeline", "show"}, bytes.NewReader(showInput), &showOutput); err != nil {
		t.Fatal(err)
	}
	var shown cli.OutputV1
	if err := json.Unmarshal(showOutput.Bytes(), &shown); err != nil || shown.Event == nil || shown.Event.EvidenceRecordRef != record.EvidenceRecordRef {
		t.Fatalf("timeline show output = %#v err=%v", shown, err)
	}

	query := continuitycontract.TimelineQuery{LedgerScopeDigest: record.LedgerScopeDigest, AuthorityWatermark: "authority-watermark", PolicyWatermark: "policy-watermark", PageLimit: 10}
	watchInput, _ := json.Marshal(cli.TimelineWatchRequestV1{Query: query})
	var watchOutput bytes.Buffer
	if err := runner.RunV1(context.Background(), []string{"timeline", "watch"}, bytes.NewReader(watchInput), &watchOutput); err != nil {
		t.Fatal(err)
	}
	var watched cli.OutputV1
	if err := json.Unmarshal(watchOutput.Bytes(), &watched); err != nil || watched.Page == nil || len(watched.Page.Records) != 1 || watched.Page.NextCursor == "" {
		t.Fatalf("timeline watch output = %#v err=%v", watched, err)
	}

	checkpointInput, _ := json.Marshal(manifest.Ref())
	var checkpointOutput bytes.Buffer
	if err := runner.RunV1(context.Background(), []string{"checkpoint", "inspect"}, bytes.NewReader(checkpointInput), &checkpointOutput); err != nil {
		t.Fatal(err)
	}
	var inspected cli.OutputV1
	if err := json.Unmarshal(checkpointOutput.Bytes(), &inspected); err != nil || inspected.Checkpoint == nil || !inspected.Checkpoint.Ref().Exact().Equal(manifest.Ref().Exact()) {
		t.Fatalf("checkpoint inspect output = %#v err=%v", inspected, err)
	}
}

func TestRunnerV1RejectsMismatchUnknownFieldsAndWritesNoPartialOutput(t *testing.T) {
	now := time.Unix(1_900_000_000, 0)
	port := &cliWorkflowPortV1{}
	runner, _ := cli.NewRunnerV1(port)
	request := cliWorkflowRequestV1(now, appcontract.ContinuityRestoreV1)
	payload, _ := json.Marshal(request)
	for _, test := range []struct {
		args    []string
		payload []byte
	}{{[]string{"checkpoint", "create"}, payload}, {[]string{"checkpoint", "delete"}, payload}, {[]string{"restore"}, append(payload[:len(payload)-1], []byte(`,"permit":"caller"}`)...)}} {
		var output bytes.Buffer
		if err := runner.RunV1(context.Background(), test.args, bytes.NewReader(test.payload), &output); err == nil || output.Len() != 0 {
			t.Fatalf("invalid command/input succeeded or wrote output: %v err=%v output=%q", test.args, err, output.String())
		}
	}
	if port.submitCalls != 0 {
		t.Fatalf("rejected requests reached gateway %d times", port.submitCalls)
	}
}

func TestRunnerV1ReadCommandsRejectMissingPortAndUnknownFields(t *testing.T) {
	runner, err := cli.NewRunnerV1(&cliWorkflowPortV1{})
	if err != nil {
		t.Fatal(err)
	}
	for _, args := range [][]string{{"timeline", "show"}, {"timeline", "watch"}, {"checkpoint", "inspect"}} {
		var output bytes.Buffer
		if err := runner.RunV1(context.Background(), args, bytes.NewBufferString(`{}`), &output); err == nil || output.Len() != 0 {
			t.Fatalf("missing read port accepted for %v: err=%v output=%q", args, err, output.String())
		}
	}

	now := time.Date(2026, 7, 18, 10, 30, 0, 0, time.UTC)
	backend := memory.NewWithClock(func() time.Time { return now })
	clock := &testkit.Clock{Time: now}
	timeline, err := domain.NewReferenceTimeline(backend, clock, time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	checkpoint, err := domain.NewCheckpointManifestControllerV2(backend)
	if err != nil {
		t.Fatal(err)
	}
	client, err := sdk.New(sdk.Config{Timeline: timeline, Checkpoints: checkpoint, Clock: func() time.Time { return now }})
	if err != nil {
		t.Fatal(err)
	}
	runner, err = cli.NewRunnerWithReadersV1(&cliWorkflowPortV1{}, client, client)
	if err != nil {
		t.Fatal(err)
	}
	for _, test := range []struct {
		args    []string
		payload string
	}{
		{[]string{"timeline", "show"}, `{"evidence_record_ref":"record-1","trusted":true}`},
		{[]string{"timeline", "watch"}, `{"query":{},"trusted":true}`},
		{[]string{"checkpoint", "inspect"}, `{"manifest_id":"manifest-1","trusted":true}`},
	} {
		var output bytes.Buffer
		if err := runner.RunV1(context.Background(), test.args, bytes.NewBufferString(test.payload), &output); err == nil || output.Len() != 0 {
			t.Fatalf("unknown read field accepted for %v: err=%v output=%q", test.args, err, output.String())
		}
	}
}

func TestRunnerV1TypedNilCanceledAndConcurrentDeterministic(t *testing.T) {
	var typedNil *cliWorkflowPortV1
	if _, err := cli.NewRunnerV1(typedNil); err == nil {
		t.Fatal("typed-nil workflow port was accepted")
	}
	now := time.Unix(1_900_000_000, 0)
	port := &cliWorkflowPortV1{}
	runner, _ := cli.NewRunnerV1(port)
	request := cliWorkflowRequestV1(now, appcontract.ContinuityTimelineProjectV1)
	payload, _ := json.Marshal(request)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := runner.RunV1(ctx, []string{"timeline", "project"}, bytes.NewReader(payload), &bytes.Buffer{}); err != context.Canceled {
		t.Fatalf("canceled context was not preserved: %v", err)
	}
	const workers = 64
	var wait sync.WaitGroup
	outputs := make(chan string, workers)
	errors := make(chan error, workers)
	for range workers {
		wait.Add(1)
		go func() {
			defer wait.Done()
			var output bytes.Buffer
			errors <- runner.RunV1(context.Background(), []string{"timeline", "project"}, bytes.NewReader(payload), &output)
			outputs <- output.String()
		}()
	}
	wait.Wait()
	close(outputs)
	close(errors)
	for err := range errors {
		if err != nil {
			t.Fatal(err)
		}
	}
	first := ""
	for output := range outputs {
		if first == "" {
			first = output
		} else if output != first {
			t.Fatal("concurrent CLI output drifted")
		}
	}
}

type cliWorkflowPortV1 struct {
	mu           sync.Mutex
	submitCalls  int
	inspectCalls int
}

func (p *cliWorkflowPortV1) SubmitGovernedWorkflow(_ context.Context, request appcontract.ContinuityWorkflowRequestV1) (appcontract.ContinuityWorkflowInspectionV1, error) {
	p.mu.Lock()
	p.submitCalls++
	p.mu.Unlock()
	return cliWorkflowInspectionV1(request), nil
}

func (p *cliWorkflowPortV1) InspectGovernedWorkflow(_ context.Context, request appcontract.ContinuityWorkflowRequestV1) (appcontract.ContinuityWorkflowInspectionV1, error) {
	p.mu.Lock()
	p.inspectCalls++
	p.mu.Unlock()
	return cliWorkflowInspectionV1(request), nil
}

func cliWorkflowInspectionV1(request appcontract.ContinuityWorkflowRequestV1) appcontract.ContinuityWorkflowInspectionV1 {
	digest, _ := request.DigestV1()
	ref := func(id string) appcontract.ApplicationFactRefV2 {
		return appcontract.ApplicationFactRefV2{Ref: id, Revision: 1, Digest: core.DigestBytes([]byte(id))}
	}
	return appcontract.ContinuityWorkflowInspectionV1{RequestDigest: digest, Submission: ref(request.RequestID), Command: ref(request.RequestID), Outbox: ref(request.RequestID), Plan: ref("plan-1"), Status: appcontract.WorkflowAcceptedV2, Steps: []appcontract.ContinuityWorkflowStepRefV1{{StepID: "continuity-root", Kind: runtimeports.NamespacedNameV2(request.Kind), Descriptor: appcontract.StepDescriptorRefV2{Kind: runtimeports.NamespacedNameV2(request.Kind), Revision: 1, Digest: core.DigestBytes([]byte("descriptor")), ExpiresUnixNano: request.NotAfterUnixNano}}}}
}

func cliWorkflowRequestV1(now time.Time, kind appcontract.ContinuityWorkflowKindV1) appcontract.ContinuityWorkflowRequestV1 {
	digest := func(value string) core.Digest { return core.DigestBytes([]byte(value)) }
	scope := core.ExecutionScope{Identity: core.AgentIdentityRef{TenantID: "tenant-1", ID: "agent-1", Epoch: 1}, Lineage: core.LineageRef{ID: "lineage-1", PlanDigest: digest("plan")}, Instance: core.InstanceRef{ID: "instance-1", Epoch: 1}, SandboxLease: &core.SandboxLeaseRef{ID: "lease-1", Epoch: 1}, AuthorityEpoch: 1}
	scopeDigest, _ := runtimeports.ExecutionScopeDigestV2(scope)
	ref := func(id string) appcontract.ApplicationFactRefV2 {
		return appcontract.ApplicationFactRefV2{Ref: id, Revision: 1, Digest: digest(id)}
	}
	return appcontract.ContinuityWorkflowRequestV1{ContractVersion: appcontract.ContinuityWorkflowContractVersionV1, RequestID: "request-1", IdempotencyKey: "idempotency-1", Kind: kind, Target: scope, DomainRequest: appcontract.ExternalFactRefV1{ContractVersion: "praxis.continuity/request/v1", SchemaRef: "praxis.continuity/request/v1", Owner: appcontract.ExternalOwnerBindingV1{BindingSetID: "binding-set-1", BindingSetRevision: 1, ComponentID: "praxis/continuity", ManifestDigest: digest("manifest"), ArtifactDigest: digest("artifact"), Capability: "praxis.continuity/governed-workflow", FactKind: "praxis.continuity/request"}, TenantID: scope.Identity.TenantID, ScopeDigest: scopeDigest, ID: "domain-request-1", Revision: 1, Digest: digest("domain-request")}, CompiledGraph: ref("compiled-graph"), Binding: ref("binding"), Consumer: ref("consumer"), RequestedUnixNano: now.Add(-time.Second).UnixNano(), NotAfterUnixNano: now.Add(time.Hour).UnixNano()}
}

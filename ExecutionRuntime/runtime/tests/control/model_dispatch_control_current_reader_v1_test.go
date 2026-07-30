package control_test

import (
	"context"
	"go/parser"
	"go/token"
	"reflect"
	"runtime"
	"strconv"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/runtime/control"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/ports"
)

func TestModelDispatchControlCurrentReaderV1DerivesClosedStates(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name        string
		runStatus   core.RunStatus
		desired     ports.DesiredExecutionStateV2
		command     ports.ApplicationCommandKindV2
		status      ports.ApplicationCommandStatusV2
		want        control.ModelDispatchControlStateV1
		wantCurrent bool
	}{
		{name: "running desired running", runStatus: core.RunRunning, desired: ports.DesiredRunningV2, want: control.ModelDispatchControlDispatchableV1, wantCurrent: true},
		{name: "cancel run", runStatus: core.RunRunning, desired: ports.DesiredRunningV2, command: ports.ApplicationCommandCancelRunV2, status: ports.ApplicationCommandAcceptedV2, want: control.ModelDispatchControlCancelRequestedV1},
		{name: "fence", runStatus: core.RunRunning, desired: ports.DesiredFencedV2, command: ports.ApplicationCommandFenceV2, status: ports.ApplicationCommandAcceptedV2, want: control.ModelDispatchControlFencedV1},
		{name: "revoke", runStatus: core.RunRunning, desired: ports.DesiredFencedV2, command: ports.ApplicationCommandRevokeV2, status: ports.ApplicationCommandCompletedV2, want: control.ModelDispatchControlRevokedV1},
		{name: "stop", runStatus: core.RunRunning, desired: ports.DesiredStoppedV2, command: ports.ApplicationCommandStopInstanceV2, status: ports.ApplicationCommandAcceptedV2, want: control.ModelDispatchControlIndeterminateV1},
		{name: "command indeterminate", runStatus: core.RunRunning, desired: ports.DesiredRunningV2, command: ports.ApplicationCommandResumeV2, status: ports.ApplicationCommandIndeterminateV2, want: control.ModelDispatchControlIndeterminateV1},
		{name: "command superseded", runStatus: core.RunRunning, desired: ports.DesiredRunningV2, command: ports.ApplicationCommandResumeV2, status: ports.ApplicationCommandSupersededV2, want: control.ModelDispatchControlIndeterminateV1},
		{name: "run stopping", runStatus: core.RunStopping, desired: ports.DesiredRunningV2, want: control.ModelDispatchControlIndeterminateV1},
		{name: "run terminal", runStatus: core.RunTerminal, desired: ports.DesiredRunningV2, want: control.ModelDispatchControlIndeterminateV1},
	}
	for _, testCase := range tests {
		testCase := testCase
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()
			fixture := newModelDispatchControlReaderFixtureV1(t)
			fixture.run.Status = testCase.runStatus
			switch testCase.runStatus {
			case core.RunStopping:
				fixture.run.StartedAt = fixture.now.Add(-time.Minute)
			case core.RunTerminal:
				fixture.run.StartedAt = fixture.now.Add(-time.Minute)
				fixture.run.EndedAt = fixture.now.Add(-time.Second)
				fixture.run.Outcome = core.OutcomeCancelled
			}
			fixture.desired.Desired = testCase.desired
			if testCase.command != "" {
				fixture.desired.Revision = 2
				fixture.desired.LastCommandID = "command-control-reader"
				fixture.commands = []ports.ApplicationCommandRecordV2{
					modelDispatchControlCommandRecordV1(t, fixture.now, fixture.scope, testCase.command, testCase.status, fixture.desired.LastCommandID, 2),
				}
			}
			reader := fixture.reader(t)
			projection, err := reader.InspectModelDispatchControlCurrentV1(context.Background(), fixture.operation, fixture.effectID)
			if err != nil {
				t.Fatal(err)
			}
			if projection.State != testCase.want {
				t.Fatalf("state=%s want=%s", projection.State, testCase.want)
			}
			err = projection.ValidateCurrent(fixture.operation, fixture.effectID, fixture.run, fixture.now)
			if testCase.wantCurrent && err != nil {
				t.Fatalf("dispatchable projection is not current: %v", err)
			}
			if !testCase.wantCurrent && err == nil {
				t.Fatal("non-dispatchable projection passed current validation")
			}
		})
	}
}

func TestModelDispatchControlCurrentReaderV1RejectsMissingAndMismatchedLastCommand(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name   string
		mutate func(*modelDispatchControlReaderFixtureV1)
	}{
		{name: "advanced desired missing id", mutate: func(f *modelDispatchControlReaderFixtureV1) {
			f.desired.Revision = 2
		}},
		{name: "last command absent", mutate: func(f *modelDispatchControlReaderFixtureV1) {
			f.desired.Revision = 2
			f.desired.LastCommandID = "missing-command"
		}},
		{name: "last command revision mismatch", mutate: func(f *modelDispatchControlReaderFixtureV1) {
			f.desired.Revision = 3
			f.desired.LastCommandID = "command-control-reader"
			f.commands = []ports.ApplicationCommandRecordV2{
				modelDispatchControlCommandRecordV1(t, f.now, f.scope, ports.ApplicationCommandStartV2, ports.ApplicationCommandAcceptedV2, f.desired.LastCommandID, 2),
			}
		}},
		{name: "last command scope mismatch", mutate: func(f *modelDispatchControlReaderFixtureV1) {
			f.desired.Revision = 2
			f.desired.LastCommandID = "command-control-reader"
			record := modelDispatchControlCommandRecordV1(t, f.now, f.scope, ports.ApplicationCommandStartV2, ports.ApplicationCommandAcceptedV2, f.desired.LastCommandID, 2)
			record.Envelope.Target.Instance.ID = "other-instance"
			f.commands = []ports.ApplicationCommandRecordV2{record}
		}},
		{name: "command indeterminate relation mismatch", mutate: func(f *modelDispatchControlReaderFixtureV1) {
			f.desired.Revision = 2
			f.desired.Desired = ports.DesiredRunningV2
			f.desired.LastCommandID = "command-control-reader"
			f.commands = []ports.ApplicationCommandRecordV2{
				modelDispatchControlCommandRecordV1(t, f.now, f.scope, ports.ApplicationCommandFenceV2, ports.ApplicationCommandAcceptedV2, f.desired.LastCommandID, 2),
			}
		}},
	}
	for _, testCase := range tests {
		testCase := testCase
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()
			fixture := newModelDispatchControlReaderFixtureV1(t)
			testCase.mutate(&fixture)
			projection, err := fixture.reader(t).InspectModelDispatchControlCurrentV1(context.Background(), fixture.operation, fixture.effectID)
			if err == nil || projection != (control.ModelDispatchControlCurrentProjectionV1{}) {
				t.Fatalf("mismatch did not fail closed: projection=%+v err=%v", projection, err)
			}
		})
	}
}

func TestModelDispatchControlCurrentReaderV1RejectsS1S2DriftAndClockRollback(t *testing.T) {
	t.Parallel()
	t.Run("S1 S2 drift", func(t *testing.T) {
		fixture := newModelDispatchControlReaderFixtureV1(t)
		fixture.source.afterFirstDesired = func(value ports.DesiredStateSnapshotV2) ports.DesiredStateSnapshotV2 {
			value.Desired = ports.DesiredFencedV2
			return value
		}
		projection, err := fixture.reader(t).InspectModelDispatchControlCurrentV1(context.Background(), fixture.operation, fixture.effectID)
		if !core.HasReason(err, core.ReasonBindingDrift) || projection != (control.ModelDispatchControlCurrentProjectionV1{}) {
			t.Fatalf("S1/S2 drift did not fail closed: projection=%+v err=%v", projection, err)
		}
	})
	t.Run("clock rollback", func(t *testing.T) {
		fixture := newModelDispatchControlReaderFixtureV1(t)
		fixture.clock = &modelDispatchControlClockV1{values: []time.Time{fixture.now, fixture.now.Add(-time.Nanosecond)}}
		projection, err := fixture.reader(t).InspectModelDispatchControlCurrentV1(context.Background(), fixture.operation, fixture.effectID)
		if !core.HasReason(err, core.ReasonClockRegression) || projection != (control.ModelDispatchControlCurrentProjectionV1{}) {
			t.Fatalf("clock rollback did not fail closed: projection=%+v err=%v", projection, err)
		}
	})
}

func TestModelDispatchControlCurrentReaderV1TTLTypedNilAndCancellation(t *testing.T) {
	t.Parallel()
	fixture := newModelDispatchControlReaderFixtureV1(t)
	reader := fixture.reader(t)
	projection, err := reader.InspectModelDispatchControlCurrentV1(context.Background(), fixture.operation, fixture.effectID)
	if err != nil {
		t.Fatal(err)
	}
	if projection.ExpiresUnixNano-projection.CheckedUnixNano != control.MaxModelDispatchControlCurrentTTLV1.Nanoseconds() {
		t.Fatalf("TTL is not bounded exactly: %+v", projection)
	}
	if err := projection.ValidateCurrent(fixture.operation, fixture.effectID, fixture.run, time.Unix(0, projection.ExpiresUnixNano)); err == nil {
		t.Fatal("projection remained current at NotAfter")
	}
	var nilRuns *modelDispatchControlSourceV1
	if _, err := control.NewModelDispatchControlCurrentReaderV1(nilRuns, fixture.source, fixture.clock.Now); !core.HasReason(err, core.ReasonComponentMissing) {
		t.Fatalf("typed-nil Run reader accepted: %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	value, err := reader.InspectModelDispatchControlCurrentV1(ctx, fixture.operation, fixture.effectID)
	if err == nil || value != (control.ModelDispatchControlCurrentProjectionV1{}) {
		t.Fatalf("cancelled context returned current projection: value=%+v err=%v", value, err)
	}
}

func TestModelDispatchControlCurrentReaderV1ConcurrentReadsAreReadOnly(t *testing.T) {
	t.Parallel()
	fixture := newModelDispatchControlReaderFixtureV1(t)
	reader := fixture.reader(t)
	const workers = 64
	var wg sync.WaitGroup
	var failures atomic.Int32
	digests := make(chan core.Digest, workers)
	for range workers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			projection, err := reader.InspectModelDispatchControlCurrentV1(context.Background(), fixture.operation, fixture.effectID)
			if err != nil {
				failures.Add(1)
				return
			}
			digests <- projection.ProjectionDigest
		}()
	}
	wg.Wait()
	close(digests)
	var expected core.Digest
	for digest := range digests {
		if expected == "" {
			expected = digest
		}
		if digest != expected {
			t.Fatalf("concurrent read returned different projection digest: got=%s want=%s", digest, expected)
		}
	}
	if failures.Load() != 0 || fixture.source.writeCalls.Load() != 0 {
		t.Fatalf("concurrent reads failed or wrote facts: failures=%d writes=%d", failures.Load(), fixture.source.writeCalls.Load())
	}
	readerType := reflect.TypeOf((*control.ModelDispatchControlCurrentReaderV1)(nil)).Elem()
	if readerType.NumMethod() != 1 || readerType.Method(0).Name != "InspectModelDispatchControlCurrentV1" {
		t.Fatalf("Reader capability widened: %s", readerType)
	}
}

func TestModelDispatchControlCurrentReaderV1PropagatesReadFailuresWithoutWrites(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name   string
		mutate func(*modelDispatchControlSourceV1)
	}{
		{name: "Run unavailable", mutate: func(source *modelDispatchControlSourceV1) {
			source.runErr = core.NewError(core.ErrorUnavailable, core.ReasonEvidenceUnavailable, "Run unavailable")
		}},
		{name: "desired indeterminate", mutate: func(source *modelDispatchControlSourceV1) {
			source.desiredErr = core.NewError(core.ErrorIndeterminate, core.ReasonEvidenceUnavailable, "desired current is indeterminate")
		}},
		{name: "commands unavailable", mutate: func(source *modelDispatchControlSourceV1) {
			source.commandsErr = core.NewError(core.ErrorUnavailable, core.ReasonEvidenceUnavailable, "command history unavailable")
		}},
	}
	for _, testCase := range tests {
		testCase := testCase
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()
			fixture := newModelDispatchControlReaderFixtureV1(t)
			testCase.mutate(fixture.source)
			value, err := fixture.reader(t).InspectModelDispatchControlCurrentV1(context.Background(), fixture.operation, fixture.effectID)
			if err == nil || value != (control.ModelDispatchControlCurrentProjectionV1{}) {
				t.Fatalf("read failure leaked a projection: value=%+v err=%v", value, err)
			}
			if fixture.source.writeCalls.Load() != 0 {
				t.Fatalf("read failure caused %d writes", fixture.source.writeCalls.Load())
			}
		})
	}
}

func TestModelDispatchControlCurrentReaderV1ImportBoundary(t *testing.T) {
	t.Parallel()
	_, currentFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	source := currentFile[:len(currentFile)-len("tests/control/model_dispatch_control_current_reader_v1_test.go")] + "control/model_dispatch_control_current_reader_v1.go"
	parsed, err := parser.ParseFile(token.NewFileSet(), source, nil, parser.ImportsOnly)
	if err != nil {
		t.Fatal(err)
	}
	allowed := map[string]struct{}{
		"context": {}, "reflect": {}, "sort": {}, "strings": {}, "time": {},
		"github.com/Proview-China/rax/ExecutionRuntime/runtime/core":  {},
		"github.com/Proview-China/rax/ExecutionRuntime/runtime/ports": {},
	}
	for _, imported := range parsed.Imports {
		path, err := strconv.Unquote(imported.Path.Value)
		if err != nil {
			t.Fatal(err)
		}
		if _, exists := allowed[path]; !exists {
			t.Fatalf("Model dispatch-control adapter imported forbidden owner package %q", path)
		}
	}
}

type modelDispatchControlReaderFixtureV1 struct {
	now       time.Time
	scope     core.ExecutionScope
	operation ports.OperationSubjectV3
	effectID  core.EffectIntentID
	run       core.AgentRunRecord
	desired   ports.DesiredStateSnapshotV2
	commands  []ports.ApplicationCommandRecordV2
	source    *modelDispatchControlSourceV1
	clock     *modelDispatchControlClockV1
}

func newModelDispatchControlReaderFixtureV1(t *testing.T) modelDispatchControlReaderFixtureV1 {
	t.Helper()
	now := time.Date(2026, 7, 30, 20, 0, 0, 0, time.UTC)
	lease := &core.SandboxLeaseRef{ID: "lease-control-reader", Epoch: 2}
	scope := core.ExecutionScope{
		Identity: core.AgentIdentityRef{TenantID: "tenant-control-reader", ID: "agent-control-reader", Epoch: 3},
		Lineage:  core.LineageRef{ID: "lineage-control-reader", PlanDigest: controlReaderDigestV1(t, "plan")},
		Instance: core.InstanceRef{ID: "instance-control-reader", Epoch: 4}, SandboxLease: lease, AuthorityEpoch: 5,
	}
	scopeDigest, err := ports.ExecutionScopeDigestV2(scope)
	if err != nil {
		t.Fatal(err)
	}
	operation := ports.OperationSubjectV3{
		Kind: ports.OperationScopeRunV3, ExecutionScope: scope, ExecutionScopeDigest: scopeDigest, RunID: "run-control-reader",
		SubjectRevision: 1, CurrentProjectionRef: "run-current-control-reader", CurrentProjectionRevision: 1,
		CurrentProjectionDigest: controlReaderDigestV1(t, "run-current"),
	}
	run := core.AgentRunRecord{ID: operation.RunID, Scope: scope, Status: core.RunRunning, Revision: 4, StartedAt: now.Add(-time.Minute)}
	desired := ports.DesiredStateSnapshotV2{Scope: scope, Desired: ports.DesiredRunningV2, Revision: 1}
	source := &modelDispatchControlSourceV1{run: run, desired: desired}
	return modelDispatchControlReaderFixtureV1{
		now: now, scope: scope, operation: operation, effectID: "effect-control-reader",
		run: run, desired: desired, source: source,
		clock: &modelDispatchControlClockV1{values: []time.Time{now, now, now}},
	}
}

func (f *modelDispatchControlReaderFixtureV1) reader(t *testing.T) control.ModelDispatchControlCurrentReaderV1 {
	t.Helper()
	f.source.run = f.run
	f.source.desired = f.desired
	f.source.commands = append([]ports.ApplicationCommandRecordV2(nil), f.commands...)
	reader, err := control.NewModelDispatchControlCurrentReaderV1(f.source, f.source, f.clock.Now)
	if err != nil {
		t.Fatal(err)
	}
	return reader
}

type modelDispatchControlSourceV1 struct {
	mu                sync.Mutex
	run               core.AgentRunRecord
	desired           ports.DesiredStateSnapshotV2
	commands          []ports.ApplicationCommandRecordV2
	desiredReads      int
	afterFirstDesired func(ports.DesiredStateSnapshotV2) ports.DesiredStateSnapshotV2
	runErr            error
	desiredErr        error
	commandsErr       error
	writeCalls        atomic.Int32
}

func (s *modelDispatchControlSourceV1) InspectRun(context.Context, core.ExecutionScope, core.AgentRunID) (core.AgentRunRecord, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.runErr != nil {
		return core.AgentRunRecord{}, s.runErr
	}
	return s.run, nil
}

func (s *modelDispatchControlSourceV1) ReadDesiredState(context.Context, core.ExecutionScope) (ports.DesiredStateSnapshotV2, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.desiredErr != nil {
		return ports.DesiredStateSnapshotV2{}, s.desiredErr
	}
	s.desiredReads++
	value := s.desired
	if s.desiredReads > 1 && s.afterFirstDesired != nil {
		value = s.afterFirstDesired(value)
	}
	return value, nil
}

func (s *modelDispatchControlSourceV1) ListCommands(context.Context, core.ExecutionScope) ([]ports.ApplicationCommandRecordV2, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.commandsErr != nil {
		return nil, s.commandsErr
	}
	return append([]ports.ApplicationCommandRecordV2(nil), s.commands...), nil
}

type modelDispatchControlClockV1 struct {
	mu     sync.Mutex
	values []time.Time
	index  int
}

func (c *modelDispatchControlClockV1) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	if len(c.values) == 0 {
		return time.Time{}
	}
	if c.index >= len(c.values) {
		return c.values[len(c.values)-1]
	}
	value := c.values[c.index]
	c.index++
	return value
}

func modelDispatchControlCommandRecordV1(
	t *testing.T,
	now time.Time,
	scope core.ExecutionScope,
	kind ports.ApplicationCommandKindV2,
	status ports.ApplicationCommandStatusV2,
	id string,
	revision core.Revision,
) ports.ApplicationCommandRecordV2 {
	t.Helper()
	leaseEpoch := scope.SandboxLease.Epoch
	envelope := ports.ApplicationCommandEnvelopeV2{
		ID: id, Kind: kind, Target: scope, Actor: "runtime-control-reader", AuthorityRef: "authority-control-reader",
		CanonicalPayloadDigest: controlReaderDigestV1(t, "command-"+id), IdempotencyKey: "idem-" + id,
		Preconditions: core.ExecutionPreconditions{
			IdentityEpoch: scope.Identity.Epoch, InstanceEpoch: scope.Instance.Epoch, LeaseEpoch: &leaseEpoch,
			AuthorityEpoch: scope.AuthorityEpoch, Revision: revision - 1,
		},
		SubmittedAt: now.Add(-2 * time.Minute), ExpiresAt: now.Add(time.Minute),
	}
	if kind == ports.ApplicationCommandApproveEffectV2 || kind == ports.ApplicationCommandDenyEffectV2 {
		envelope.EffectIntentID = "effect-control-reader"
		envelope.EffectIntentRevision = 1
	}
	return ports.ApplicationCommandRecordV2{Envelope: envelope, Revision: revision, Status: status, RecordedAt: now.Add(-time.Minute)}
}

func controlReaderDigestV1(t *testing.T, value string) core.Digest {
	t.Helper()
	digest, err := core.DigestJSON(value)
	if err != nil {
		t.Fatal(err)
	}
	return digest
}

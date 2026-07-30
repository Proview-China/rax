package dataplaneadapter_test

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	runtimecore "github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	runtimeports "github.com/Proview-China/rax/ExecutionRuntime/runtime/ports"
	"github.com/Proview-China/rax/ExecutionRuntime/sandbox/contract"
	"github.com/Proview-China/rax/ExecutionRuntime/sandbox/dataplaneadapter"
	sandboxports "github.com/Proview-China/rax/ExecutionRuntime/sandbox/ports"
	"github.com/Proview-China/rax/ExecutionRuntime/sandbox/runtimeadapter"
)

func TestWorkspaceReadCurrentV1SealsExactS1S2Projection(t *testing.T) {
	t.Parallel()
	fixture := newWorkspaceReadCurrentFixtureV1(t)
	got, err := fixture.adapter.InspectWorkspaceReadCurrentV1(context.Background(), fixture.query)
	if err != nil {
		t.Fatal(err)
	}
	if err := got.ValidateCurrent(fixture.now); err != nil {
		t.Fatal(err)
	}
	if got.Association != fixture.association.Ref ||
		got.Command != fixture.command.Meta.Ref() ||
		got.WorkspaceView != fixture.workspace.Meta.Ref() ||
		got.RuntimeEnforcementDigest != fixture.current.Digest ||
		got.ExpiresUnixNano != fixture.current.Phase.ExpiresUnixNano {
		t.Fatalf("projection lost exact coordinates or natural TTL: %#v", got)
	}
	if fixture.readers.runtimeCalls.Load() != 2 ||
		fixture.readers.associationCalls.Load() != 2 ||
		fixture.readers.commandCalls.Load() != 2 ||
		fixture.readers.workspaceCalls.Load() != 2 {
		t.Fatalf("S1/S2 did not independently reread every Owner: %#v", fixture.readers)
	}
}

func TestWorkspaceReadCurrentV1RejectsCallerSubstitutedAuthorizationProof(t *testing.T) {
	t.Parallel()
	fixture := newWorkspaceReadCurrentFixtureV1(t)
	tests := map[string]func(*sandboxports.WorkspaceReadCurrentQueryV1){
		"stable key": func(q *sandboxports.WorkspaceReadCurrentQueryV1) {
			q.StableKeyDigest = runtimecore.DigestBytes([]byte("other-stable-key"))
		},
		"authorization digest": func(q *sandboxports.WorkspaceReadCurrentQueryV1) {
			q.AuthorizationDigest = runtimecore.DigestBytes([]byte("other-authorization"))
		},
		"association": func(q *sandboxports.WorkspaceReadCurrentQueryV1) {
			q.Association.ID = "other-association"
		},
		"domain command": func(q *sandboxports.WorkspaceReadCurrentQueryV1) {
			q.DomainCommand.ID = "other-command"
		},
	}
	for name, mutate := range tests {
		t.Run(name, func(t *testing.T) {
			query := fixture.query
			mutate(&query)
			query, err := sandboxports.SealWorkspaceReadCurrentQueryV1(query)
			if err != nil {
				if fixture.readers.runtimeCalls.Load() != 0 {
					t.Fatal("invalid authorization proof reached the Runtime current reader")
				}
				return
			}
			if _, err = fixture.adapter.InspectWorkspaceReadCurrentV1(context.Background(), query); err == nil {
				t.Fatal("caller-substituted authorization coordinate was accepted")
			}
			if fixture.readers.runtimeCalls.Load() != 0 {
				t.Fatal("invalid authorization proof reached the Runtime current reader")
			}
		})
	}
}

func TestWorkspaceReadDispatchConstructorDoesNotReadWallClockForExactQuery(t *testing.T) {
	t.Parallel()
	fixture := newWorkspaceReadCurrentFixtureV1(t)
	legacy := fixture.current.Dispatch.Record.Permit.LegacyPermit
	request, err := dataplaneadapter.NewDispatchRequestV1(dataplaneadapter.DispatchInput{
		RequestID: "workspace-current-request-v1", Current: fixture.current,
		WorkspaceReadCurrent: &fixture.query, EffectKind: "praxis.sandbox/workspace-read",
		PayloadSchema: legacy.PayloadSchema.Key(), PayloadRevision: uint64(legacy.PayloadRevision),
		Payload: fixture.payload, RequestedNotAfter: time.Unix(0, fixture.current.Phase.ExpiresUnixNano),
	})
	if err != nil {
		t.Fatal(err)
	}
	if request.RuntimeCurrentQueryDigest == "" {
		t.Fatal("exact workspace read current query was not sealed into the dispatch request")
	}
}

func TestWorkspaceReadCurrentV1ConcurrentExactQueryIsDeterministicAndReadOnly(t *testing.T) {
	t.Parallel()
	fixture := newWorkspaceReadCurrentFixtureV1(t)
	var ticks atomic.Int64
	adapter, err := runtimeadapter.NewWorkspaceReadCurrentAdapterV1(
		fixture.readers, fixture.readers, fixture.readers, fixture.readers,
		func() time.Time { return fixture.now.Add(time.Duration(ticks.Add(1)) * time.Nanosecond) },
	)
	if err != nil {
		t.Fatal(err)
	}
	const workers = 64
	var wg sync.WaitGroup
	type resultDigest struct {
		semantic   string
		projection string
	}
	digests := make(chan resultDigest, workers)
	errs := make(chan error, workers)
	for range workers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			got, err := adapter.InspectWorkspaceReadCurrentV1(context.Background(), fixture.query)
			if err != nil {
				errs <- err
				return
			}
			digests <- resultDigest{semantic: got.SemanticDigest, projection: got.ProjectionDigest}
		}()
	}
	wg.Wait()
	close(digests)
	close(errs)
	for err := range errs {
		t.Fatal(err)
	}
	var expected string
	projections := make(map[string]struct{}, workers)
	for digest := range digests {
		if expected == "" {
			expected = digest.semantic
		} else if digest.semantic != expected {
			t.Fatalf("concurrent exact query returned semantic digest %q, want %q", digest.semantic, expected)
		}
		projections[digest.projection] = struct{}{}
	}
	if len(projections) < 2 {
		t.Fatal("changing read clocks did not produce distinct full ProjectionDigests")
	}
	if fixture.readers.runtimeCalls.Load() != workers*2 ||
		fixture.readers.associationCalls.Load() != workers*2 ||
		fixture.readers.commandCalls.Load() != workers*2 ||
		fixture.readers.workspaceCalls.Load() != workers*2 {
		t.Fatal("concurrent exact reads did not remain a two-pass, read-only operation")
	}
}

func TestWorkspaceReadCurrentV1AllowsFreshRuntimeEnvelopeDigest(t *testing.T) {
	t.Parallel()
	fixture := newWorkspaceReadCurrentFixtureV1(t)
	fresh := fixture.current
	fresh.CheckedUnixNano++
	fresh.Dispatch.CheckedUnixNano++
	var err error
	fresh, err = runtimeports.SealCurrentOperationDispatchEnforcementV4(fresh)
	if err != nil {
		t.Fatal(err)
	}
	fixture.readers.runtimeSecond = fresh
	var ticks atomic.Int64
	adapter, err := runtimeadapter.NewWorkspaceReadCurrentAdapterV1(
		fixture.readers, fixture.readers, fixture.readers, fixture.readers,
		func() time.Time { return fixture.now.Add(time.Duration(ticks.Add(1)) * time.Nanosecond) },
	)
	if err != nil {
		t.Fatal(err)
	}
	got, err := adapter.InspectWorkspaceReadCurrentV1(context.Background(), fixture.query)
	if err != nil {
		t.Fatal(err)
	}
	if got.RuntimeEnforcementDigest != fresh.Digest {
		t.Fatal("projection did not preserve the fresh S2 Runtime envelope digest")
	}
}

func TestWorkspaceReadCurrentV1RejectsValidSemanticRuntimeDrift(t *testing.T) {
	t.Parallel()
	fixture := newWorkspaceReadCurrentFixtureV1(t)
	drifted := fixture.current
	drifted.Sandbox.RuntimeLease.FenceEpoch++
	var err error
	drifted.Sandbox, err = runtimeports.SealOperationDispatchSandboxCurrentProjectionV4(drifted.Sandbox)
	if err != nil {
		t.Fatal(err)
	}
	drifted, err = runtimeports.SealCurrentOperationDispatchEnforcementV4(drifted)
	if err != nil {
		t.Fatal(err)
	}
	fixture.readers.runtimeSecond = drifted
	if got, err := fixture.adapter.InspectWorkspaceReadCurrentV1(context.Background(), fixture.query); err == nil || got.ProjectionDigest != "" {
		t.Fatalf("valid semantic Runtime drift returned projection=%#v err=%v", got, err)
	}
}

func TestWorkspaceReadCurrentV1FailsClosedOnEveryOwnerAxis(t *testing.T) {
	t.Parallel()
	tests := map[string]func(*workspaceReadCurrentReadersV1){
		"association lost reply": func(r *workspaceReadCurrentReadersV1) { r.associationErr = errors.New("lost association reply") },
		"command lost reply":     func(r *workspaceReadCurrentReadersV1) { r.commandErr = errors.New("lost command reply") },
		"workspace lost reply":   func(r *workspaceReadCurrentReadersV1) { r.workspaceErr = errors.New("lost workspace reply") },
		"runtime lost reply":     func(r *workspaceReadCurrentReadersV1) { r.runtimeErr = errors.New("lost Runtime reply") },
		"association drift": func(r *workspaceReadCurrentReadersV1) {
			r.associationSecond = r.association
			r.associationSecond.ExpiresUnixNano--
		},
		"command drift": func(r *workspaceReadCurrentReadersV1) {
			r.commandSecond = r.command
			r.commandSecond.RelativePath = "src/other.txt"
		},
		"workspace scope drift": func(r *workspaceReadCurrentReadersV1) {
			r.workspaceSecond = r.workspace
			r.workspaceSecond.ReadScopes = []string{"other"}
		},
		"workspace fence drift": func(r *workspaceReadCurrentReadersV1) {
			r.workspaceSecond = r.workspace
			r.workspaceSecond.Lease.FenceEpoch++
		},
		"runtime enforcement drift": func(r *workspaceReadCurrentReadersV1) {
			r.runtimeSecond = r.current
			r.runtimeSecond.ExpiresUnixNano--
		},
	}
	for name, mutate := range tests {
		t.Run(name, func(t *testing.T) {
			fixture := newWorkspaceReadCurrentFixtureV1(t)
			mutate(fixture.readers)
			got, err := fixture.adapter.InspectWorkspaceReadCurrentV1(context.Background(), fixture.query)
			if err == nil || got.ProjectionDigest != "" {
				t.Fatalf("drift returned projection=%#v err=%v", got, err)
			}
		})
	}
}

func TestWorkspaceReadCurrentV1AdapterRebuildReturnsSameProjection(t *testing.T) {
	t.Parallel()
	fixture := newWorkspaceReadCurrentFixtureV1(t)
	first, err := fixture.adapter.InspectWorkspaceReadCurrentV1(context.Background(), fixture.query)
	if err != nil {
		t.Fatal(err)
	}
	rebuilt, err := runtimeadapter.NewWorkspaceReadCurrentAdapterV1(
		fixture.readers, fixture.readers, fixture.readers, fixture.readers, func() time.Time { return fixture.now },
	)
	if err != nil {
		t.Fatal(err)
	}
	second, err := rebuilt.InspectWorkspaceReadCurrentV1(context.Background(), fixture.query)
	if err != nil {
		t.Fatal(err)
	}
	if first != second {
		t.Fatalf("rebuilt read adapter returned a different exact projection\nfirst=%#v\nsecond=%#v", first, second)
	}
}

type workspaceReadCurrentFixtureV1 struct {
	now         time.Time
	current     runtimeports.CurrentOperationDispatchEnforcementV4
	association runtimeports.PreparedDomainCommandAssociationCurrentProjectionV1
	command     contract.WorkspaceReadCommandV1
	workspace   contract.WorkspaceView
	payload     dataplaneadapter.ProviderPayloadV1
	query       sandboxports.WorkspaceReadCurrentQueryV1
	readers     *workspaceReadCurrentReadersV1
	adapter     *runtimeadapter.WorkspaceReadCurrentAdapterV1
}

func newWorkspaceReadCurrentFixtureV1(t *testing.T) workspaceReadCurrentFixtureV1 {
	t.Helper()
	// Deliberately far beyond the host wall clock: pure constructors must not
	// read time.Now, while CurrentServer/Reader validate with their injected clock.
	now := time.Unix(4_000_000_000, 0).UTC()
	workspaceDigest := "sha256:" + digestWorkspaceReadCurrentV1("workspace")
	fileScopeDigest := string(runtimecore.DigestBytes([]byte("workspace-read-file-scope")))
	payload, err := dataplaneadapter.NewWorkspaceReadPayloadV1(dataplaneadapter.WorkspaceReadPayloadV1{
		WorkspaceBindingID: "workspace-current-v1", WorkspaceDigest: workspaceDigest,
		Workspace: dataplaneadapter.ExactRefV1{
			ID: "workspace-current-v1", Revision: 1, Digest: workspaceDigest,
			ExpiresUnixNano: now.Add(time.Hour).UnixNano(),
		},
		FileScopeDigest: fileScopeDigest, RelativePath: "src/main.txt", MaxBytes: 4096, S1Checked: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	payloadDigest, err := digestWorkspaceReadExecutorIPC("ProviderPayloadV1", payload)
	if err != nil {
		t.Fatal(err)
	}
	current := buildWorkspaceReadRuntimeCurrentV4(t, now, payloadDigest)
	expires := time.Unix(0, current.Sandbox.RuntimeLease.Ref.ExpiresUnixNano)
	workspace := contract.WorkspaceView{
		Meta: contract.Meta{
			ContractVersion: contract.ContractFamily, ID: "workspace-current-v1", Revision: 1,
			Digest: digestWorkspaceReadCurrentV1("workspace"), CreatedUnixNano: now.UnixNano(),
			UpdatedUnixNano: now.UnixNano(), ExpiresUnixNano: expires.UnixNano(),
		},
		BaseArtifactRef: contract.Ref{ID: "base", Revision: 1, Digest: digestWorkspaceReadCurrentV1("base")},
		BaseRevision:    "main",
		OverlayRef:      contract.Ref{ID: "overlay", Revision: 1, Digest: digestWorkspaceReadCurrentV1("overlay")},
		PolicyRef:       contract.Ref{ID: "policy", Revision: 1, Digest: digestWorkspaceReadCurrentV1("policy")},
		Lease: contract.RuntimeLeaseBinding{
			TenantID:   string(current.Sandbox.Operation.ExecutionScope.Identity.TenantID),
			InstanceID: string(current.Sandbox.RuntimeLease.Instance.ID), InstanceEpoch: uint64(current.Sandbox.RuntimeLease.Instance.Epoch),
			LeaseID: string(current.Sandbox.RuntimeLease.Lease.ID), LeaseEpoch: uint64(current.Sandbox.RuntimeLease.Lease.Epoch),
			FenceEpoch: uint64(current.Sandbox.RuntimeLease.FenceEpoch), ScopeDigest: string(current.Sandbox.RuntimeLease.ScopeDigest),
			ObservedRevision: uint64(current.Sandbox.RuntimeLease.ObservedRevision), ExpiresUnixNano: expires.UnixNano(),
		},
		ReadScopes: []string{"src"}, WriteScopes: []string{"src"},
		FileScopeDigest: fileScopeDigest,
	}
	legacy := current.Dispatch.Record.Permit.LegacyPermit
	legacyDigest, err := legacy.DigestV3()
	if err != nil {
		t.Fatal(err)
	}
	delegation := runtimeports.ExecutionDelegationRefV2{ID: "delegation-workspace-current-v1", Revision: 1, Digest: runtimecore.DigestBytes([]byte("delegation-workspace-current-v1"))}
	preparedID, err := runtimeports.DerivePreparedProviderAttemptIDV2(delegation.ID, legacy.ID, legacy.AttemptID)
	if err != nil {
		t.Fatal(err)
	}
	prepared, err := runtimeports.SealPreparedProviderAttemptRefV2(runtimeports.PreparedProviderAttemptRefV2{
		ID: preparedID, Revision: 1, DeclaredDelegation: delegation,
		OperationDigest: current.Sandbox.OperationDigest, IntentID: current.Sandbox.EffectID,
		IntentRevision: current.Sandbox.IntentRevision, IntentDigest: current.Sandbox.IntentDigest,
		PermitID: legacy.ID, PermitRevision: legacy.Revision, PermitDigest: legacyDigest,
		AttemptID: current.Sandbox.AttemptID, Provider: current.Sandbox.ProviderBinding,
		PayloadSchema: legacy.PayloadSchema, PayloadDigest: legacy.PayloadDigest, PayloadRevision: legacy.PayloadRevision,
		PreparedUnixNano: now.UnixNano(), ExpiresUnixNano: expires.UnixNano(),
	})
	if err != nil {
		t.Fatal(err)
	}
	attempt := runtimeports.OperationDispatchAttemptRefV3{
		OperationDigest: current.Sandbox.OperationDigest, EffectID: current.Sandbox.EffectID,
		IntentRevision: current.Sandbox.IntentRevision, IntentDigest: current.Sandbox.IntentDigest,
		PermitID: legacy.ID, PermitRevision: legacy.Revision, PermitDigest: legacyDigest,
		AttemptID: current.Sandbox.AttemptID, Delegation: &delegation,
	}
	dispatchDigest, err := runtimecore.CanonicalJSONDigest("praxis.sandbox.workspace-read", "1.0.0", "OperationDispatchAttemptRefV3", attempt)
	if err != nil {
		t.Fatal(err)
	}
	commandDraft := contract.WorkspaceReadCommandV1{
		TenantID:                string(current.Sandbox.Operation.ExecutionScope.Identity.TenantID),
		SourceToolCommand:       contract.Ref{ID: "tool-command", Revision: 1, Digest: digestWorkspaceReadCurrentV1("tool-command")},
		SourceToolPayloadSchema: legacy.PayloadSchema.Key(), SourceToolPayloadDigest: string(legacy.PayloadDigest), SourceToolPayloadRevision: uint64(legacy.PayloadRevision),
		WorkspaceView: workspace.Meta.Ref(), FileScopeDigest: workspace.FileScopeDigest, RelativePath: "src/main.txt", MaxBytes: 4096,
		RequestedNotAfterUnixNano: expires.UnixNano(), OperationDigest: string(current.Sandbox.OperationDigest),
		EffectID: string(current.Sandbox.EffectID), IntentRevision: uint64(current.Sandbox.IntentRevision), IntentDigest: string(current.Sandbox.IntentDigest),
		AttemptID: current.Sandbox.AttemptID, PreparedDigest: string(prepared.Digest), DispatchDigest: string(dispatchDigest),
		ProviderComponent: string(current.Sandbox.ProviderBinding.ComponentID), ProviderManifest: string(current.Sandbox.ProviderBinding.ManifestDigest),
	}
	command, err := contract.SealWorkspaceReadCommandV1(commandDraft, "workspace-current-command-v1", now, expires)
	if err != nil {
		t.Fatal(err)
	}
	domainCommand := runtimeports.OperationDomainCommandRefV1{
		Owner: runtimeports.EffectOwnerRefV2{
			Role: runtimeports.OwnerSettlement, ComponentID: current.Sandbox.ProviderBinding.ComponentID,
			ManifestDigest: current.Sandbox.ProviderBinding.ManifestDigest,
		},
		Kind: runtimeports.NamespacedNameV2(contract.WorkspaceReadCommandKindV1),
		ID:   command.Meta.ID, Revision: runtimecore.Revision(command.Meta.Revision), Digest: runtimecore.Digest("sha256:" + command.Meta.Digest),
	}
	association, err := runtimeports.SealPreparedDomainCommandAssociationCurrentProjectionV1(runtimeports.PreparedDomainCommandAssociationCurrentProjectionV1{
		Operation: current.Sandbox.Operation, OperationDigest: current.Sandbox.OperationDigest,
		EffectID: current.Sandbox.EffectID, EffectRevision: current.Sandbox.IntentRevision, IntentDigest: current.Sandbox.IntentDigest,
		Prepared: prepared, Attempt: attempt, Provider: current.Sandbox.ProviderBinding,
		PayloadSchema: legacy.PayloadSchema, PayloadDigest: legacy.PayloadDigest, PayloadRevision: legacy.PayloadRevision,
		DomainCommand: domainCommand, CheckedUnixNano: now.UnixNano(), ExpiresUnixNano: expires.UnixNano(),
	})
	if err != nil {
		t.Fatal(err)
	}
	runtimeInspect := runtimeports.InspectCurrentOperationDispatchEnforcementRequestV4{
		Inspect: runtimeports.InspectOperationDispatchEnforcementRequestV4{
			Operation: current.Sandbox.Operation, EffectID: current.Sandbox.EffectID,
			PermitID: current.Phase.PermitID, Phase: current.Phase.Phase,
		},
		PermitDigest: current.Phase.PermitDigest, AdmissionDigest: current.Phase.AdmissionDigest,
		ReviewAuthorization: current.Phase.ReviewAuthorization, SandboxAttempt: current.Phase.SandboxAttempt,
		SandboxProjectionDigest: current.Sandbox.ProjectionDigest,
	}
	transport := runtimeports.ProviderBindingRefV2{
		BindingSetID: "transport-workspace-current-v1", BindingSetRevision: 1,
		ComponentID: "praxis.runtime/transport", ManifestDigest: runtimecore.DigestBytes([]byte("transport-manifest")),
		ArtifactDigest: runtimecore.DigestBytes([]byte("transport-artifact")),
		Capability:     runtimeports.ControlledOperationProviderTransportCapabilityV2,
	}
	authorization, err := runtimeports.SealControlledOperationPhysicalExecutionAuthorizationV3(runtimeports.ControlledOperationPhysicalExecutionAuthorizationV3{
		UnifiedNotAfterUnixNano: expires.UnixNano(), ProviderTransport: transport, Provider: current.Sandbox.ProviderBinding,
		Operation: current.Sandbox.Operation, OperationDigest: current.Sandbox.OperationDigest,
		OperationScopeDigest: current.Sandbox.Operation.ExecutionScopeDigest,
		EffectKind:           runtimeports.OperationScopeEvidenceActionEffectKindV3, Prepared: prepared, Attempt: attempt,
		ExecuteEnforcement: current.Phase,
		ExecuteEvidenceHandoff: runtimeports.OperationScopeEvidenceProviderHandoffRefV3{
			ID: "handoff-workspace-current-v1", Revision: 1, Digest: runtimecore.DigestBytes([]byte("handoff-workspace-current-v1")),
			ExpiresUnixNano: expires.UnixNano(),
		},
		Boundary: runtimeports.OperationProviderBoundaryRefV1{
			ID: "boundary-workspace-current-v1", Revision: 1, Digest: runtimecore.DigestBytes([]byte("boundary-workspace-current-v1")),
		},
		Association: association.Ref, DomainCommand: domainCommand,
	})
	if err != nil {
		t.Fatal(err)
	}
	query, err := sandboxports.SealWorkspaceReadCurrentQueryV1(sandboxports.WorkspaceReadCurrentQueryV1{
		RuntimeInspect: runtimeInspect, Authorization: authorization, StableKeyDigest: authorization.StableKeyDigest,
		AuthorizationDigest: authorization.AuthorizationDigest,
		Association:         association.Ref, DomainCommand: domainCommand, Command: command.Meta.Ref(),
		WorkspaceView: workspace.Meta.Ref(), FileScopeDigest: workspace.FileScopeDigest, RelativePath: command.RelativePath,
		CheckedUnixNano: now.UnixNano(), ExpiresUnixNano: expires.UnixNano(),
	})
	if err != nil {
		t.Fatal(err)
	}
	readers := &workspaceReadCurrentReadersV1{current: current, association: association, command: command, workspace: workspace}
	adapter, err := runtimeadapter.NewWorkspaceReadCurrentAdapterV1(readers, readers, readers, readers, func() time.Time { return now })
	if err != nil {
		t.Fatal(err)
	}
	return workspaceReadCurrentFixtureV1{
		now: now, current: current, association: association, command: command, workspace: workspace,
		payload: payload, query: query, readers: readers, adapter: adapter,
	}
}

type workspaceReadCurrentReadersV1 struct {
	current           runtimeports.CurrentOperationDispatchEnforcementV4
	runtimeSecond     runtimeports.CurrentOperationDispatchEnforcementV4
	association       runtimeports.PreparedDomainCommandAssociationCurrentProjectionV1
	associationSecond runtimeports.PreparedDomainCommandAssociationCurrentProjectionV1
	command           contract.WorkspaceReadCommandV1
	commandSecond     contract.WorkspaceReadCommandV1
	workspace         contract.WorkspaceView
	workspaceSecond   contract.WorkspaceView
	runtimeErr        error
	associationErr    error
	commandErr        error
	workspaceErr      error
	runtimeCalls      atomic.Int64
	associationCalls  atomic.Int64
	commandCalls      atomic.Int64
	workspaceCalls    atomic.Int64
}

func (r *workspaceReadCurrentReadersV1) InspectCurrentOperationDispatchEnforcementV4(context.Context, runtimeports.InspectCurrentOperationDispatchEnforcementRequestV4) (runtimeports.CurrentOperationDispatchEnforcementV4, error) {
	call := r.runtimeCalls.Add(1)
	if r.runtimeErr != nil {
		return runtimeports.CurrentOperationDispatchEnforcementV4{}, r.runtimeErr
	}
	if call%2 == 0 && r.runtimeSecond.Digest != "" {
		return r.runtimeSecond, nil
	}
	return r.current, nil
}

func (r *workspaceReadCurrentReadersV1) InspectCurrentPreparedDomainCommandAssociationV1(context.Context, runtimeports.PreparedDomainCommandAssociationRefV1) (runtimeports.PreparedDomainCommandAssociationCurrentProjectionV1, error) {
	call := r.associationCalls.Add(1)
	if r.associationErr != nil {
		return runtimeports.PreparedDomainCommandAssociationCurrentProjectionV1{}, r.associationErr
	}
	if call%2 == 0 && r.associationSecond.Ref.ID != "" {
		return r.associationSecond, nil
	}
	return r.association, nil
}

func (r *workspaceReadCurrentReadersV1) InspectWorkspaceReadCommandCurrentV1(context.Context, contract.Ref) (contract.WorkspaceReadCommandV1, error) {
	call := r.commandCalls.Add(1)
	if r.commandErr != nil {
		return contract.WorkspaceReadCommandV1{}, r.commandErr
	}
	if call%2 == 0 && r.commandSecond.Meta.ID != "" {
		return r.commandSecond, nil
	}
	return r.command, nil
}

func (r *workspaceReadCurrentReadersV1) InspectWorkspaceViewCurrentV1(context.Context, contract.Ref) (contract.WorkspaceView, error) {
	call := r.workspaceCalls.Add(1)
	if r.workspaceErr != nil {
		return contract.WorkspaceView{}, r.workspaceErr
	}
	if call%2 == 0 && r.workspaceSecond.Meta.ID != "" {
		return r.workspaceSecond, nil
	}
	return r.workspace, nil
}

func (*workspaceReadCurrentReadersV1) InspectWorkspaceChangeSetCurrentV1(context.Context, contract.Ref) (contract.WorkspaceChangeSet, error) {
	return contract.WorkspaceChangeSet{}, errors.New("not used")
}

func digestWorkspaceReadCurrentV1(value string) string {
	return string(runtimecore.DigestBytes([]byte(value)))[len("sha256:"):]
}

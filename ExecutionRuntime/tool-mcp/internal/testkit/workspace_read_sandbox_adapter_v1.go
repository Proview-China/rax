package testkit

import (
	"context"
	"encoding/json"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	runtimecore "github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	runtimeports "github.com/Proview-China/rax/ExecutionRuntime/runtime/ports"
	sandboxcontract "github.com/Proview-China/rax/ExecutionRuntime/sandbox/contract"
	sandboxports "github.com/Proview-China/rax/ExecutionRuntime/sandbox/ports"
	toolcontract "github.com/Proview-China/rax/ExecutionRuntime/tool-mcp/contract"
)

type WorkspaceReadSandboxFixtureV1 struct {
	Now               time.Time
	Input             toolcontract.WorkspaceReadInputV1
	SourceCommand     toolcontract.ObjectRef
	PayloadSchema     runtimeports.SchemaRefV2
	PayloadDigest     runtimecore.Digest
	CanonicalInput    []byte
	Command           sandboxcontract.WorkspaceReadCommandV1
	Authorization     runtimeports.ControlledOperationPhysicalExecutionAuthorizationV3
	Admission         runtimeports.ControlledOperationProviderAdmissionReceiptRefV2
	RuntimeInspection toolcontract.WorkspaceReadRuntimeAdmissionInspectionV1
	Binding           sandboxports.WorkspaceReadAdmissionAttemptBindingV1
	Projection        sandboxcontract.WorkspaceReadExecutionProjectionV1
	Envelope          sandboxports.WorkspaceReadInspectionEnvelopeV2
}

func WorkspaceReadSandboxV1(now time.Time) WorkspaceReadSandboxFixtureV1 {
	return WorkspaceReadSandboxWithContentV1(now, "Praxis")
}

func WorkspaceReadSandboxWithContentV1(now time.Time, content string) WorkspaceReadSandboxFixtureV1 {
	return workspaceReadSandboxFixtureV1(now, content, "")
}

func WorkspaceReadSandboxWithExpectedFileDigestV1(
	now time.Time,
	content string,
	expectedFileDigest string,
) WorkspaceReadSandboxFixtureV1 {
	return workspaceReadSandboxFixtureV1(now, content, expectedFileDigest)
}

func workspaceReadSandboxFixtureV1(now time.Time, content, expectedFileDigest string) WorkspaceReadSandboxFixtureV1 {
	expires := now.Add(10 * time.Second)
	base := MCPExecutionV1(now)
	workspace := sandboxcontract.Ref{
		ID: "workspace-view-tool-adapter-v1", Revision: 1, Digest: SandboxDigestV1("workspace-view-tool-adapter-v1"),
	}
	input := toolcontract.WorkspaceReadInputV1{
		WorkspaceRoot: toolcontract.WorkspaceExactRefV1{
			ID: workspace.ID, Revision: runtimecore.Revision(workspace.Revision), Digest: runtimecore.Digest("sha256:" + workspace.Digest),
		},
		RelativePath: "src/main.txt", StartByte: 0, MaxBytes: uint64(len(content)), RequestedNotAfter: expires.UnixNano(),
	}
	canonicalInput, err := json.Marshal(input)
	if err != nil {
		panic(err)
	}
	payloadSchema := Schema("workspace-read-input")
	payloadDigest := runtimecore.DigestBytes(canonicalInput)
	sourceCommand := toolcontract.ObjectRef{ID: "tool-workspace-read-v1", Revision: 1, Digest: Digest("tool-workspace-read-v1")}
	fileID, err := sandboxcontract.WorkspaceReadFileIDV1(workspace.ID, input.RelativePath)
	if err != nil {
		panic(err)
	}
	var expectedFile *sandboxcontract.Ref
	if expectedFileDigest != "" {
		expectedFile = &sandboxcontract.Ref{
			ID: fileID, Revision: workspace.Revision, Digest: expectedFileDigest,
		}
	}
	prepared := base.Authorization.Prepared
	prepared.PayloadSchema = payloadSchema
	prepared.PayloadDigest = payloadDigest
	prepared.PayloadRevision = 1
	prepared.Digest = ""
	prepared, err = runtimeports.SealPreparedProviderAttemptRefV2(prepared)
	if err != nil {
		panic(err)
	}
	base.Authorization.Prepared = prepared
	attemptDigest, err := runtimecore.CanonicalJSONDigest(
		"praxis.sandbox.workspace-read", "1.0.0", "OperationDispatchAttemptRefV3", base.Authorization.Attempt,
	)
	if err != nil {
		panic(err)
	}
	command, err := sandboxcontract.SealWorkspaceReadCommandV1(sandboxcontract.WorkspaceReadCommandV1{
		TenantID:                  string(base.Authorization.Operation.ExecutionScope.Identity.TenantID),
		SourceToolCommand:         sandboxcontract.Ref{ID: sourceCommand.ID, Revision: uint64(sourceCommand.Revision), Digest: strings.TrimPrefix(string(sourceCommand.Digest), "sha256:")},
		SourceToolPayloadSchema:   payloadSchema.Key(),
		SourceToolPayloadDigest:   string(payloadDigest),
		SourceToolPayloadRevision: 1,
		WorkspaceView:             workspace,
		FileScopeDigest:           SandboxDigestV1("workspace-read-file-scope-v1"),
		RelativePath:              input.RelativePath,
		StartByte:                 input.StartByte,
		MaxBytes:                  input.MaxBytes,
		ExpectedFileRef:           expectedFile,
		RequestedNotAfterUnixNano: input.RequestedNotAfter,
		OperationDigest:           string(base.Authorization.OperationDigest),
		EffectID:                  string(base.Authorization.Attempt.EffectID),
		IntentRevision:            uint64(base.Authorization.Attempt.IntentRevision),
		IntentDigest:              string(base.Authorization.Attempt.IntentDigest),
		AttemptID:                 base.Authorization.Attempt.AttemptID,
		PreparedDigest:            string(base.Authorization.Prepared.Digest),
		DispatchDigest:            string(attemptDigest),
		ProviderComponent:         string(base.Authorization.Provider.ComponentID),
		ProviderManifest:          string(base.Authorization.Provider.ManifestDigest),
	}, "workspace-read-command-tool-adapter-v1", now, expires)
	if err != nil {
		panic(err)
	}
	domainCommand := runtimeports.OperationDomainCommandRefV1{
		Owner: runtimeports.EffectOwnerRefV2{
			Role: runtimeports.OwnerSettlement, ComponentID: base.Authorization.Provider.ComponentID,
			ManifestDigest: base.Authorization.Provider.ManifestDigest,
		},
		Kind: runtimeports.NamespacedNameV2(sandboxcontract.WorkspaceReadCommandKindV1),
		ID:   command.Meta.ID, Revision: runtimecore.Revision(command.Meta.Revision),
		Digest: runtimecore.Digest("sha256:" + command.Meta.Digest),
	}
	authorization := base.Authorization
	authorization.DomainCommand = domainCommand
	authorization.AuthorizationDigest = ""
	authorization.StableKeyDigest = ""
	authorization, err = runtimeports.SealControlledOperationPhysicalExecutionAuthorizationV3(authorization)
	if err != nil {
		panic(err)
	}
	admission, err := runtimeports.SealControlledOperationProviderAdmissionReceiptRefV2(runtimeports.ControlledOperationProviderAdmissionReceiptRefV2{
		ID: "workspace-read-admission-tool-adapter-v1", Revision: 1,
		StableKeyDigest: authorization.StableKeyDigest, Admitted: true,
	})
	if err != nil {
		panic(err)
	}
	admissionBinding := sandboxcontract.WorkspaceReadReceiptBindingV1{
		ID: admission.ID, Revision: uint64(admission.Revision), Digest: string(admission.Digest),
		StableKeyDigest: string(admission.StableKeyDigest), CheckedUnixNano: now.UnixNano(), ExpiresUnixNano: expires.UnixNano(),
	}
	ttl, err := sandboxcontract.SealWorkspaceReadTTLClosureV1(sandboxcontract.WorkspaceReadTTLClosureV1{
		UnifiedNotAfterUnixNano:       expires.UnixNano(),
		RuntimeEnforcementExpiresNano: expires.UnixNano(),
		AssociationExpiresUnixNano:    expires.UnixNano(),
		CommandRequestedNotAfterNano:  expires.UnixNano(),
		CommandExpiresUnixNano:        expires.UnixNano(),
		WorkspaceViewExpiresUnixNano:  expires.UnixNano(),
		WorkspaceLeaseExpiresUnixNano: expires.UnixNano(),
	})
	if err != nil {
		panic(err)
	}
	reservation, err := sandboxcontract.SealWorkspaceReadReservationV1(sandboxcontract.WorkspaceReadReservationV1{
		StableKeyDigest: string(authorization.StableKeyDigest), AuthorizationDigest: string(authorization.AuthorizationDigest),
		RequestDigest: command.Meta.Digest, PayloadDigest: command.SourceToolPayloadDigest,
		Command: command.Meta.Ref(), WorkspaceView: workspace, AttemptID: "workspace-read-attempt-tool-adapter-v1", TTLClosure: ttl,
	}, "workspace-read-reservation-tool-adapter-v1", now, expires)
	if err != nil {
		panic(err)
	}
	origin, err := sandboxcontract.SealWorkspaceReadAttemptV1(sandboxcontract.WorkspaceReadAttemptV1{
		StableKeyDigest: string(authorization.StableKeyDigest), RequestDigest: command.Meta.Digest,
		PayloadDigest: command.SourceToolPayloadDigest, Reservation: reservation.Meta.Ref(),
		AdmissionReceipt: admissionBinding, State: sandboxcontract.WorkspaceReadStartedV1,
	}, "workspace-read-attempt-tool-adapter-v1", 1, now, expires)
	if err != nil {
		panic(err)
	}
	providerReceipt := sandboxcontract.WorkspaceReadReceiptBindingV1{
		ID: "workspace-read-provider-receipt-v1", Revision: 1, Digest: SandboxDigestV1("workspace-read-provider-receipt-v1"),
		ObservationDigest: SandboxDigestV1("workspace-read-provider-observation-v1"),
		StableKeyDigest:   string(admission.StableKeyDigest), CheckedUnixNano: now.UnixNano(), ExpiresUnixNano: expires.UnixNano(),
	}
	observation, err := sandboxcontract.SealWorkspaceReadObservationV1(sandboxcontract.WorkspaceReadObservationV1{
		Reservation: reservation.Meta.Ref(), Command: command.Meta.Ref(), WorkspaceView: workspace,
		File:         sandboxcontract.Ref{ID: fileID, Revision: workspace.Revision, Digest: SandboxDigestV1("workspace-file-tool-adapter-v1")},
		RelativePath: input.RelativePath, StartByte: input.StartByte, ReturnedBytes: uint64(len(content)), TotalBytes: uint64(len(content)),
		Complete: true, Content: content,
		ContentDigest:     sandboxcontract.WorkspaceReadContentDigestV1([]byte(content), input.StartByte, uint64(len(content)), true),
		S1CheckedUnixNano: now.UnixNano(), S2CheckedUnixNano: now.UnixNano(),
		AdmissionReceipt: admissionBinding, ProviderReceipt: providerReceipt,
	}, "workspace-read-observation-tool-adapter-v1", now, expires)
	if err != nil {
		panic(err)
	}
	observedRef := observation.Meta.Ref()
	terminal, err := sandboxcontract.SealWorkspaceReadAttemptV1(sandboxcontract.WorkspaceReadAttemptV1{
		StableKeyDigest: string(authorization.StableKeyDigest), RequestDigest: command.Meta.Digest,
		PayloadDigest: command.SourceToolPayloadDigest, Reservation: reservation.Meta.Ref(),
		AdmissionReceipt: admissionBinding, State: sandboxcontract.WorkspaceReadObservedV1, Observation: &observedRef,
	}, origin.Meta.ID, 2, now, expires)
	if err != nil {
		panic(err)
	}
	projection := sandboxcontract.WorkspaceReadExecutionProjectionV1{
		Attempt: terminal, Reservation: reservation, AdmissionReceipt: admissionBinding,
		Observation: &observation, ProviderReceipt: &providerReceipt,
	}
	if err = projection.ValidateShape(); err != nil {
		panic(err)
	}
	envelope, err := sandboxports.SealWorkspaceReadInspectionEnvelopeV2(sandboxports.WorkspaceReadInspectionEnvelopeV2{
		RequestedOriginAttemptRef: sandboxcontract.WorkspaceReadAttemptRefV1{
			ID: origin.Meta.ID, Revision: origin.Meta.Revision, Digest: origin.Meta.Digest,
		},
		CurrentProjection: projection,
		CheckedUnixNano:   now.UnixNano(),
		ExpiresUnixNano:   expires.UnixNano(),
	})
	if err != nil {
		panic(err)
	}
	binding, err := sandboxports.SealWorkspaceReadAdmissionAttemptBindingV1(sandboxports.WorkspaceReadAdmissionAttemptBindingV1{
		AdmissionReceipt: admission,
		Attempt: sandboxcontract.WorkspaceReadAttemptRefV1{
			ID: origin.Meta.ID, Revision: origin.Meta.Revision, Digest: origin.Meta.Digest,
		},
		Command: command.Meta.Ref(), AuthorizationDigest: authorization.AuthorizationDigest,
		StableKeyDigest: authorization.StableKeyDigest, Association: authorization.Association,
		DomainCommand: authorization.DomainCommand, CreatedUnixNano: now.UnixNano(), ExpiresUnixNano: expires.UnixNano(),
	})
	if err != nil {
		panic(err)
	}
	runtimeInspection, err := toolcontract.SealWorkspaceReadRuntimeAdmissionInspectionV1(
		toolcontract.WorkspaceReadRuntimeAdmissionInspectionV1{
			Attempt:         authorization.Attempt,
			Admission:       admission,
			CheckedUnixNano: now.UnixNano(),
			ExpiresUnixNano: expires.UnixNano(),
		},
	)
	if err != nil {
		panic(err)
	}
	return WorkspaceReadSandboxFixtureV1{
		Now: now, Input: input, SourceCommand: sourceCommand, PayloadSchema: payloadSchema,
		PayloadDigest: payloadDigest, CanonicalInput: canonicalInput, Command: command, Authorization: authorization,
		Admission: admission, RuntimeInspection: runtimeInspection, Binding: binding, Projection: projection, Envelope: envelope,
	}
}

func SandboxDigestV1(label string) string {
	value, err := sandboxcontract.Digest("tool-test-fixture", struct{ Label string }{label})
	if err != nil {
		panic(err)
	}
	return value
}

type WorkspaceReadSandboxPortV1 struct {
	Command                sandboxcontract.WorkspaceReadCommandV1
	Admission              runtimeports.ControlledOperationProviderAdmissionReceiptRefV2
	Binding                sandboxports.WorkspaceReadAdmissionAttemptBindingV1
	Projection             sandboxcontract.WorkspaceReadExecutionProjectionV1
	Envelope               sandboxports.WorkspaceReadInspectionEnvelopeV2
	RuntimeInspection      toolcontract.WorkspaceReadRuntimeAdmissionInspectionV1
	ExecuteErr             error
	CommandErr             error
	HistoricalCommandErr   error
	RuntimeInspectionErr   error
	BindingErr             error
	InspectionErr          error
	OnExecute              func()
	OnInspect              func()
	authorization          sync.Mutex
	Executed               runtimeports.ControlledOperationPhysicalExecutionAuthorizationV3
	CommandCalls           atomic.Int64
	HistoricalCommandCalls atomic.Int64
	ExecuteCalls           atomic.Int64
	PhysicalReads          atomic.Int64
	BindingCalls           atomic.Int64
	InspectCalls           atomic.Int64
	RuntimeInspectionCalls atomic.Int64
}

func (p *WorkspaceReadSandboxPortV1) InspectWorkspaceReadCommandCurrentV1(context.Context, sandboxcontract.Ref) (sandboxcontract.WorkspaceReadCommandV1, error) {
	p.CommandCalls.Add(1)
	return p.Command, p.CommandErr
}

func (p *WorkspaceReadSandboxPortV1) InspectWorkspaceReadCommandExactV1(
	context.Context,
	sandboxcontract.Ref,
) (sandboxcontract.WorkspaceReadCommandV1, error) {
	p.HistoricalCommandCalls.Add(1)
	return p.Command, p.HistoricalCommandErr
}

func (p *WorkspaceReadSandboxPortV1) ExecuteControlledOperationPhysicalV3(_ context.Context, authorization runtimeports.ControlledOperationPhysicalExecutionAuthorizationV3) (runtimeports.ControlledOperationProviderAdmissionReceiptRefV2, error) {
	p.ExecuteCalls.Add(1)
	p.PhysicalReads.CompareAndSwap(0, 1)
	p.authorization.Lock()
	p.Executed = authorization
	p.authorization.Unlock()
	if p.OnExecute != nil {
		p.OnExecute()
	}
	return p.Admission, p.ExecuteErr
}

func (p *WorkspaceReadSandboxPortV1) ExecutedAuthorizationV1() runtimeports.ControlledOperationPhysicalExecutionAuthorizationV3 {
	p.authorization.Lock()
	defer p.authorization.Unlock()
	return p.Executed
}

func (p *WorkspaceReadSandboxPortV1) InspectWorkspaceReadAttemptForAdmissionV1(context.Context, runtimeports.ControlledOperationProviderAdmissionReceiptRefV2) (sandboxports.WorkspaceReadAdmissionAttemptBindingV1, error) {
	p.BindingCalls.Add(1)
	return p.Binding, p.BindingErr
}

func (p *WorkspaceReadSandboxPortV1) InspectBoundedWorkspaceReadV1(context.Context, sandboxcontract.WorkspaceReadAttemptRefV1) (sandboxcontract.WorkspaceReadExecutionProjectionV1, error) {
	p.InspectCalls.Add(1)
	return p.Projection, p.InspectionErr
}

func (p *WorkspaceReadSandboxPortV1) InspectBoundedWorkspaceReadV2(context.Context, sandboxcontract.WorkspaceReadAttemptRefV1) (sandboxports.WorkspaceReadInspectionEnvelopeV2, error) {
	p.InspectCalls.Add(1)
	if p.OnInspect != nil {
		p.OnInspect()
	}
	if p.InspectionErr != nil {
		return sandboxports.WorkspaceReadInspectionEnvelopeV2{}, p.InspectionErr
	}
	envelope := p.Envelope
	envelope.CurrentProjection = p.Projection
	sealed, err := sandboxports.SealWorkspaceReadInspectionEnvelopeV2(envelope)
	if err != nil {
		return sandboxports.WorkspaceReadInspectionEnvelopeV2{}, err
	}
	return sealed, nil
}

func (p *WorkspaceReadSandboxPortV1) InspectWorkspaceReadAdmissionForRuntimeAttemptV1(
	context.Context,
	runtimeports.OperationDispatchAttemptRefV3,
) (toolcontract.WorkspaceReadRuntimeAdmissionInspectionV1, error) {
	p.RuntimeInspectionCalls.Add(1)
	return p.RuntimeInspection, p.RuntimeInspectionErr
}

func WorkspaceReadSandboxPortFromFixtureV1(fixture WorkspaceReadSandboxFixtureV1) *WorkspaceReadSandboxPortV1 {
	return &WorkspaceReadSandboxPortV1{
		Command: fixture.Command, Admission: fixture.Admission, Binding: fixture.Binding, Projection: fixture.Projection,
		Envelope: fixture.Envelope, RuntimeInspection: fixture.RuntimeInspection,
	}
}

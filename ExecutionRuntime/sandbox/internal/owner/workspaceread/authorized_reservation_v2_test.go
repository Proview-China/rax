package workspaceread_test

import (
	"context"
	"database/sql"
	"errors"
	"path/filepath"
	"testing"
	"time"

	runtimecore "github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	runtimeports "github.com/Proview-China/rax/ExecutionRuntime/runtime/ports"
	"github.com/Proview-China/rax/ExecutionRuntime/sandbox/contract"
	ownerworkspaceread "github.com/Proview-China/rax/ExecutionRuntime/sandbox/internal/owner/workspaceread"
	sandboxports "github.com/Proview-China/rax/ExecutionRuntime/sandbox/ports"
	sqlitestore "github.com/Proview-China/rax/ExecutionRuntime/sandbox/storage/sqlite"
)

// Negative owner-seam fixtures are not public authority. Positive creation
// evidence must enter through WorkspaceReadPhysicalExecutorV1.

func TestOwnerInternalWorkspaceReadV2ReservationCommandSplicesWriteNothing(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*authorizedFixtureV2)
	}{
		{name: "workspace-view", mutate: func(f *authorizedFixtureV2) {
			f.reservation.WorkspaceView.ID = "spliced-view"
		}},
		{name: "request-digest", mutate: func(f *authorizedFixtureV2) {
			f.reservation.RequestDigest = string(digestV2("spliced-request"))
		}},
		{name: "payload-digest", mutate: func(f *authorizedFixtureV2) {
			f.reservation.PayloadDigest = string(digestV2("spliced-payload"))
		}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			ctx := context.Background()
			now := time.Unix(1_951_000_300, 0)
			expires := now.Add(time.Minute)
			database := filepath.Join(t.TempDir(), "sandbox.db")
			store, err := sqlitestore.OpenWithClock(ctx, database, func() time.Time { return now })
			if err != nil {
				t.Fatal(err)
			}
			defer store.Close()
			fixture := newAuthorizedFixtureV2(t, store, now, expires, "owner-"+test.name, "runtime-"+test.name, delegationRefV2("delegation-"+test.name))
			test.mutate(&fixture)
			fixture.reservation, err = contract.SealWorkspaceReadReservationV1(
				fixture.reservation, fixture.reservation.Meta.ID, now, expires,
			)
			if err != nil {
				t.Fatal(err)
			}
			fixture.attempt.Reservation = fixture.reservation.Meta.Ref()
			fixture.attempt.RequestDigest = fixture.reservation.RequestDigest
			fixture.attempt.PayloadDigest = fixture.reservation.PayloadDigest
			fixture.attempt, err = contract.SealWorkspaceReadAttemptV1(
				fixture.attempt, fixture.attempt.Meta.ID, 1, now, expires,
			)
			if err != nil {
				t.Fatal(err)
			}
			fixture.bindingV1.Attempt = attemptRefV2(fixture.attempt)
			fixture.bindingV1, err = sandboxports.SealWorkspaceReadAdmissionAttemptBindingV1(fixture.bindingV1)
			if err != nil {
				t.Fatal(err)
			}
			request := issueOwnerRequestV2(t, fixture, now)
			if _, _, err = store.ReserveWorkspaceReadAuthorizedV2(ctx, request); !errors.Is(err, sandboxports.ErrConflict) {
				t.Fatalf("splice accepted: %v", err)
			}
			raw := openRawV2(t, database)
			defer raw.Close()
			for _, table := range []string{
				"workspace_read_reservation",
				"workspace_read_attempt_origin",
				"workspace_read_admission_attempt_binding",
				"workspace_read_runtime_attempt_admission_binding_v2",
			} {
				var rows int
				if err = raw.QueryRowContext(ctx, "SELECT COUNT(*) FROM "+table).Scan(&rows); err != nil || rows != 0 {
					t.Fatalf("%s writes=%d err=%v", table, rows, err)
				}
			}
		})
	}
}

type authorizedFixtureV2 struct {
	authorization runtimeports.ControlledOperationPhysicalExecutionAuthorizationV3
	reservation   contract.WorkspaceReadReservationV1
	attempt       contract.WorkspaceReadAttemptV1
	bindingV1     sandboxports.WorkspaceReadAdmissionAttemptBindingV1
}

func issueOwnerRequestV2(t *testing.T, fixture authorizedFixtureV2, now time.Time) ownerworkspaceread.AuthorizedReservationV2 {
	t.Helper()
	request, err := ownerworkspaceread.NewAuthorizedReservationV2(
		fixture.reservation, fixture.attempt, fixture.bindingV1, fixture.authorization, now,
	)
	if err != nil {
		t.Fatal(err)
	}
	return request
}

func newAuthorizedFixtureV2(
	t *testing.T,
	store *sqlitestore.Store,
	now, expires time.Time,
	ownerSuffix, runtimeSuffix string,
	attemptDelegation *runtimeports.ExecutionDelegationRefV2,
) authorizedFixtureV2 {
	t.Helper()
	scope := runtimecore.ExecutionScope{
		Identity: runtimecore.AgentIdentityRef{
			TenantID: "tenant", ID: runtimecore.AgentIdentityID("identity-" + runtimeSuffix), Epoch: 1,
		},
		Lineage: runtimecore.LineageRef{
			ID: runtimecore.InstanceLineageID("lineage-" + runtimeSuffix), PlanDigest: digestV2("lineage-" + runtimeSuffix),
		},
		Instance:       runtimecore.InstanceRef{ID: runtimecore.AgentInstanceID("instance-" + runtimeSuffix), Epoch: 1},
		SandboxLease:   &runtimecore.SandboxLeaseRef{ID: runtimecore.SandboxLeaseID("lease-" + runtimeSuffix), Epoch: 1},
		AuthorityEpoch: 1,
	}
	scopeDigest, err := runtimeports.ExecutionScopeDigestV2(scope)
	if err != nil {
		t.Fatal(err)
	}
	operation := runtimeports.OperationSubjectV3{
		Kind: runtimeports.OperationScopeRunV3, ExecutionScope: scope, ExecutionScopeDigest: scopeDigest,
		RunID: runtimecore.AgentRunID("run-" + runtimeSuffix), SubjectRevision: 1,
		CurrentProjectionRef:    "operation-current-" + runtimeSuffix,
		CurrentProjectionDigest: digestV2("operation-current-" + runtimeSuffix), CurrentProjectionRevision: 1,
	}
	operationDigest, err := operation.DigestV3()
	if err != nil {
		t.Fatal(err)
	}
	provider := runtimeports.ProviderBindingRefV2{
		BindingSetID: "binding-" + runtimeSuffix, BindingSetRevision: 1,
		ComponentID: "praxis.sandbox/workspace-read", ManifestDigest: digestV2("provider-manifest"),
		ArtifactDigest: digestV2("provider-artifact"), Capability: runtimeports.CapabilityNameV2(runtimeports.OperationScopeEvidenceActionEffectKindV3),
	}
	transport := runtimeports.ProviderBindingRefV2{
		BindingSetID: "transport-" + runtimeSuffix, BindingSetRevision: 1,
		ComponentID: "praxis.runtime/transport", ManifestDigest: digestV2("transport-manifest"),
		ArtifactDigest: digestV2("transport-artifact"), Capability: runtimeports.ControlledOperationProviderTransportCapabilityV2,
	}
	declaredDelegation := delegationRefV2("declared-" + runtimeSuffix)
	if attemptDelegation != nil {
		copy := *attemptDelegation
		declaredDelegation = &copy
	}
	permitID := "permit-" + runtimeSuffix
	runtimeAttemptID := "runtime-attempt-" + runtimeSuffix
	preparedID, err := runtimeports.DerivePreparedProviderAttemptIDV2(declaredDelegation.ID, permitID, runtimeAttemptID)
	if err != nil {
		t.Fatal(err)
	}
	schema := runtimeports.SchemaRefV2{
		Namespace: "praxis.tool", Name: "workspace-read", Version: "1.0.0",
		MediaType: "application/json", ContentDigest: digestV2("schema"),
	}
	permitDigest, intentDigest := digestV2("permit-"+runtimeSuffix), digestV2("intent-"+runtimeSuffix)
	prepared, err := runtimeports.SealPreparedProviderAttemptRefV2(runtimeports.PreparedProviderAttemptRefV2{
		ID: preparedID, Revision: 1, DeclaredDelegation: *declaredDelegation,
		OperationDigest: operationDigest, IntentID: runtimecore.EffectIntentID("effect-" + runtimeSuffix),
		IntentRevision: 1, IntentDigest: intentDigest, PermitID: permitID, PermitRevision: 1,
		PermitDigest: permitDigest, AttemptID: runtimeAttemptID, Provider: provider,
		PayloadSchema: schema, PayloadDigest: digestV2("payload-" + runtimeSuffix), PayloadRevision: 1,
		PreparedUnixNano: now.UnixNano(), ExpiresUnixNano: expires.UnixNano(),
	})
	if err != nil {
		t.Fatal(err)
	}
	runtimeAttempt := runtimeports.OperationDispatchAttemptRefV3{
		OperationDigest: operationDigest, EffectID: runtimecore.EffectIntentID("effect-" + runtimeSuffix),
		IntentRevision: 1, IntentDigest: intentDigest, PermitID: permitID, PermitRevision: 1,
		PermitDigest: permitDigest, AttemptID: runtimeAttemptID, Delegation: attemptDelegation,
	}
	if err = runtimeAttempt.Validate(); err != nil {
		t.Fatal(err)
	}
	workspace := workspaceViewFixtureV2(now, expires, ownerSuffix)
	commandDraft := contract.WorkspaceReadCommandV1{
		TenantID:                "tenant",
		SourceToolCommand:       contract.Ref{ID: "tool-command-" + ownerSuffix, Revision: 1, Digest: string(digestV2("tool-command-" + ownerSuffix))},
		SourceToolPayloadSchema: schema.Key(), SourceToolPayloadDigest: string(prepared.PayloadDigest), SourceToolPayloadRevision: uint64(prepared.PayloadRevision),
		WorkspaceView: workspace.Meta.Ref(), FileScopeDigest: workspace.FileScopeDigest, RelativePath: "src/main.txt", MaxBytes: 5,
		RequestedNotAfterUnixNano: expires.UnixNano(), OperationDigest: string(runtimeAttempt.OperationDigest),
		EffectID: string(runtimeAttempt.EffectID), IntentRevision: uint64(runtimeAttempt.IntentRevision), IntentDigest: string(runtimeAttempt.IntentDigest),
		AttemptID: runtimeAttempt.AttemptID, PreparedDigest: string(prepared.Digest),
		ProviderComponent: string(provider.ComponentID), ProviderManifest: string(provider.ManifestDigest),
	}
	dispatchDigest, err := runtimecore.CanonicalJSONDigest(
		"praxis.sandbox.workspace-read", "1.0.0", "OperationDispatchAttemptRefV3", runtimeAttempt,
	)
	if err != nil {
		t.Fatal(err)
	}
	commandDraft.DispatchDigest = string(dispatchDigest)
	command, err := contract.SealWorkspaceReadCommandV1(commandDraft, "command-"+ownerSuffix, now, expires)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = store.CreateWorkspaceViewV1(context.Background(), workspace); err != nil {
		t.Fatal(err)
	}
	if _, err = store.CreateWorkspaceReadCommandV1(context.Background(), command); err != nil {
		t.Fatal(err)
	}
	domainCommand := runtimeports.OperationDomainCommandRefV1{
		Owner: runtimeports.EffectOwnerRefV2{
			Role: runtimeports.OwnerSettlement, ComponentID: provider.ComponentID, ManifestDigest: provider.ManifestDigest,
		},
		Kind: runtimeports.NamespacedNameV2(contract.WorkspaceReadCommandKindV1),
		ID:   command.Meta.ID, Revision: runtimecore.Revision(command.Meta.Revision), Digest: runtimecore.Digest("sha256:" + command.Meta.Digest),
	}
	association := runtimeports.PreparedDomainCommandAssociationRefV1{
		ID: "association-" + ownerSuffix, Revision: 1, Digest: digestV2("association-" + ownerSuffix),
	}
	sandboxAttempt := runtimeports.OperationDispatchSandboxFactRefV4{
		ID: runtimeAttempt.AttemptID, Revision: 1, Digest: digestV2("sandbox-attempt-" + ownerSuffix), ExpiresUnixNano: expires.UnixNano(),
	}
	enforcement := runtimeports.OperationDispatchEnforcementPhaseRefV4{
		OperationDigest: operationDigest, EffectID: runtimeAttempt.EffectID,
		PermitID: permitID, PermitFactRevision: runtimeAttempt.PermitRevision, PermitDigest: permitDigest,
		AdmissionDigest: digestV2("admission-enforcement-" + ownerSuffix),
		ReviewAuthorization: runtimeports.OperationReviewAuthorizationRefV4{
			ID: "review-authorization-" + ownerSuffix, Revision: 1, Digest: digestV2("review-authorization-" + ownerSuffix),
		},
		AttemptID: runtimeAttempt.AttemptID, SandboxAttempt: sandboxAttempt,
		Phase:         runtimeports.OperationDispatchEnforcementExecuteV4,
		ReceiptDigest: digestV2("execute-receipt-" + ownerSuffix), JournalRevision: 2,
		ValidatedUnixNano: now.UnixNano(), ExpiresUnixNano: expires.UnixNano(),
		PrepareReceiptDigest: digestV2("prepare-receipt-" + ownerSuffix), PreparedAttemptDigest: prepared.Digest,
	}
	authorization, err := runtimeports.SealControlledOperationPhysicalExecutionAuthorizationV3(
		runtimeports.ControlledOperationPhysicalExecutionAuthorizationV3{
			UnifiedNotAfterUnixNano: expires.UnixNano(), ProviderTransport: transport, Provider: provider,
			Operation: operation, OperationDigest: operationDigest, OperationScopeDigest: operation.ExecutionScopeDigest,
			EffectKind: runtimeports.OperationScopeEvidenceActionEffectKindV3, Prepared: prepared, Attempt: runtimeAttempt,
			ExecuteEnforcement: enforcement,
			ExecuteEvidenceHandoff: runtimeports.OperationScopeEvidenceProviderHandoffRefV3{
				ID: "evidence-handoff-" + ownerSuffix, Revision: 1, Digest: digestV2("evidence-handoff-" + ownerSuffix), ExpiresUnixNano: expires.UnixNano(),
			},
			Boundary: runtimeports.OperationProviderBoundaryRefV1{
				ID: "provider-boundary-" + ownerSuffix, Revision: 1, Digest: digestV2("provider-boundary-" + ownerSuffix),
			},
			Association: association, DomainCommand: domainCommand,
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	admission, err := runtimeports.SealControlledOperationProviderAdmissionReceiptRefV2(
		runtimeports.ControlledOperationProviderAdmissionReceiptRefV2{
			ID: "workspace-read-admission-" + ownerSuffix, Revision: 1,
			StableKeyDigest: authorization.StableKeyDigest, Admitted: true,
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	admissionBinding := contract.WorkspaceReadReceiptBindingV1{
		ID: admission.ID, Revision: uint64(admission.Revision), Digest: string(admission.Digest),
		StableKeyDigest: string(admission.StableKeyDigest), CheckedUnixNano: now.UnixNano(), ExpiresUnixNano: expires.UnixNano(),
	}
	ttl, err := contract.SealWorkspaceReadTTLClosureV1(contract.WorkspaceReadTTLClosureV1{
		UnifiedNotAfterUnixNano: expires.UnixNano(), RuntimeEnforcementExpiresNano: expires.UnixNano(),
		AssociationExpiresUnixNano: expires.UnixNano(), CommandRequestedNotAfterNano: expires.UnixNano(),
		CommandExpiresUnixNano: expires.UnixNano(), WorkspaceViewExpiresUnixNano: expires.UnixNano(),
		WorkspaceLeaseExpiresUnixNano: expires.UnixNano(),
	})
	if err != nil {
		t.Fatal(err)
	}
	workspaceAttemptID := "workspace-attempt-" + ownerSuffix
	reservation, err := contract.SealWorkspaceReadReservationV1(contract.WorkspaceReadReservationV1{
		StableKeyDigest: string(authorization.StableKeyDigest), AuthorizationDigest: string(authorization.AuthorizationDigest),
		RequestDigest: command.Meta.Digest, PayloadDigest: command.SourceToolPayloadDigest,
		Command: command.Meta.Ref(), WorkspaceView: workspace.Meta.Ref(), AttemptID: workspaceAttemptID, TTLClosure: ttl,
	}, "reservation-"+ownerSuffix, now, expires)
	if err != nil {
		t.Fatal(err)
	}
	attempt, err := contract.SealWorkspaceReadAttemptV1(contract.WorkspaceReadAttemptV1{
		StableKeyDigest: string(authorization.StableKeyDigest), RequestDigest: reservation.RequestDigest,
		PayloadDigest: reservation.PayloadDigest, Reservation: reservation.Meta.Ref(),
		AdmissionReceipt: admissionBinding, State: contract.WorkspaceReadStartedV1,
	}, workspaceAttemptID, 1, now, expires)
	if err != nil {
		t.Fatal(err)
	}
	bindingV1, err := sandboxports.SealWorkspaceReadAdmissionAttemptBindingV1(
		sandboxports.WorkspaceReadAdmissionAttemptBindingV1{
			AdmissionReceipt: admission, Attempt: attemptRefV2(attempt), Command: command.Meta.Ref(),
			AuthorizationDigest: authorization.AuthorizationDigest, StableKeyDigest: authorization.StableKeyDigest,
			Association: association, DomainCommand: domainCommand,
			CreatedUnixNano: attempt.Meta.CreatedUnixNano, ExpiresUnixNano: attempt.Meta.ExpiresUnixNano,
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	return authorizedFixtureV2{
		authorization: authorization, reservation: reservation, attempt: attempt, bindingV1: bindingV1,
	}
}

func workspaceViewFixtureV2(now, expires time.Time, suffix string) contract.WorkspaceView {
	return contract.WorkspaceView{
		Meta: contract.Meta{
			ContractVersion: contract.ContractFamily, ID: "workspace-" + suffix, Revision: 1,
			Digest: string(digestV2("workspace-" + suffix)), CreatedUnixNano: now.UnixNano(),
			UpdatedUnixNano: now.UnixNano(), ExpiresUnixNano: expires.UnixNano(),
		},
		BaseArtifactRef: contract.Ref{ID: "base-" + suffix, Revision: 1, Digest: string(digestV2("base-" + suffix))},
		BaseRevision:    "main",
		OverlayRef:      contract.Ref{ID: "overlay-" + suffix, Revision: 1, Digest: string(digestV2("overlay-" + suffix))},
		PolicyRef:       contract.Ref{ID: "policy-" + suffix, Revision: 1, Digest: string(digestV2("policy-" + suffix))},
		Lease: contract.RuntimeLeaseBinding{
			TenantID: "tenant", InstanceID: "instance-" + suffix, InstanceEpoch: 1,
			LeaseID: "lease-" + suffix, LeaseEpoch: 1, FenceEpoch: 1,
			ScopeDigest: string(digestV2("scope-" + suffix)), ObservedRevision: 1, ExpiresUnixNano: expires.UnixNano(),
		},
		ReadScopes: []string{"src"}, WriteScopes: []string{"src"},
		FileScopeDigest: string(digestV2("file-scope-" + suffix)),
	}
}

func delegationRefV2(id string) *runtimeports.ExecutionDelegationRefV2 {
	value := runtimeports.ExecutionDelegationRefV2{ID: id, Revision: 1, Digest: digestV2(id)}
	return &value
}

func attemptRefV2(attempt contract.WorkspaceReadAttemptV1) contract.WorkspaceReadAttemptRefV1 {
	return contract.WorkspaceReadAttemptRefV1{
		ID: attempt.Meta.ID, Revision: attempt.Meta.Revision, Digest: attempt.Meta.Digest,
	}
}

func digestV2(value string) runtimecore.Digest {
	return runtimecore.DigestBytes([]byte(value))
}

func openRawV2(t *testing.T, path string) *sql.DB {
	t.Helper()
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	return db
}

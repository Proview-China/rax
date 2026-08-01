package runtimeadapter

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"strings"
	"time"

	runtimecore "github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	runtimeports "github.com/Proview-China/rax/ExecutionRuntime/runtime/ports"
	"github.com/Proview-China/rax/ExecutionRuntime/sandbox/contract"
	"github.com/Proview-China/rax/ExecutionRuntime/sandbox/kernel"
	sandboxports "github.com/Proview-China/rax/ExecutionRuntime/sandbox/ports"
)

type WorkspaceReadCurrentAdapterV1 struct {
	runtime        sandboxports.OperationDispatchEnforcementCurrentReaderV4
	associations   runtimeports.PreparedDomainCommandAssociationCurrentReaderV1
	commands       sandboxports.WorkspaceReadCommandCurrentReaderV1
	publishedOwner *kernel.WorkspaceReadCommandOwnerV2
	workspaces     sandboxports.WorkspaceCurrentReaderV1
	now            func() time.Time
}

// WorkspaceReadPublishedCurrentAdapterV2 is the only V1 aggregate that may be
// promoted to the V2 Rust actual-point current. Its marker is package-private,
// so a raw Command reader or an external wrapper cannot claim this status.
type WorkspaceReadPublishedCurrentAdapterV2 struct {
	base *WorkspaceReadCurrentAdapterV1
}

func (a *WorkspaceReadPublishedCurrentAdapterV2) workspaceReadPublishedCurrentV2() bool {
	return a != nil && a.base != nil && a.base.publishedOwner != nil
}

func (a *WorkspaceReadPublishedCurrentAdapterV2) InspectWorkspaceReadCurrentV1(
	ctx context.Context,
	query sandboxports.WorkspaceReadCurrentQueryV1,
) (sandboxports.WorkspaceReadCurrentProjectionV1, error) {
	if !a.workspaceReadPublishedCurrentV2() {
		return sandboxports.WorkspaceReadCurrentProjectionV1{}, errors.New("workspace read published current adapter is unavailable")
	}
	return a.base.InspectWorkspaceReadCurrentV1(ctx, query)
}

func NewWorkspaceReadPublishedCurrentAdapterV2(
	runtime sandboxports.OperationDispatchEnforcementCurrentReaderV4,
	associations runtimeports.PreparedDomainCommandAssociationCurrentReaderV1,
	owner *kernel.WorkspaceReadCommandOwnerV2,
	workspaces sandboxports.WorkspaceCurrentReaderV1,
	now func() time.Time,
) (*WorkspaceReadPublishedCurrentAdapterV2, error) {
	if owner == nil {
		return nil, errors.New("workspace read published current adapter requires the Sandbox Command Owner")
	}
	base, err := NewWorkspaceReadCurrentAdapterV1(runtime, associations, owner, workspaces, now)
	if err != nil {
		return nil, err
	}
	base.publishedOwner = owner
	return &WorkspaceReadPublishedCurrentAdapterV2{base: base}, nil
}

func NewWorkspaceReadCurrentAdapterV1(
	runtime sandboxports.OperationDispatchEnforcementCurrentReaderV4,
	associations runtimeports.PreparedDomainCommandAssociationCurrentReaderV1,
	commands sandboxports.WorkspaceReadCommandCurrentReaderV1,
	workspaces sandboxports.WorkspaceCurrentReaderV1,
	now func() time.Time,
) (*WorkspaceReadCurrentAdapterV1, error) {
	if runtime == nil || associations == nil || commands == nil || workspaces == nil || now == nil {
		return nil, errors.New("workspace read current adapter requires all exact readers and a clock")
	}
	return &WorkspaceReadCurrentAdapterV1{
		runtime: runtime, associations: associations, commands: commands, workspaces: workspaces, now: now,
	}, nil
}

var _ sandboxports.WorkspaceReadCurrentProjectionReaderV1 = (*WorkspaceReadCurrentAdapterV1)(nil)

type workspaceReadCurrentSnapshotV1 struct {
	association    runtimeports.PreparedDomainCommandAssociationCurrentProjectionV1
	command        contract.WorkspaceReadCommandV1
	commandCurrent contract.WorkspaceReadCommandOwnerCurrentV2
	workspace      contract.WorkspaceView
	runtime        runtimeports.CurrentOperationDispatchEnforcementV4
}

func (a *WorkspaceReadCurrentAdapterV1) InspectWorkspaceReadCurrentV1(ctx context.Context, query sandboxports.WorkspaceReadCurrentQueryV1) (sandboxports.WorkspaceReadCurrentProjectionV1, error) {
	s1Checked := a.now()
	if err := query.ValidateCurrent(s1Checked); err != nil {
		return sandboxports.WorkspaceReadCurrentProjectionV1{}, err
	}
	s1, err := a.readSnapshotV1(ctx, query, s1Checked)
	if err != nil {
		return sandboxports.WorkspaceReadCurrentProjectionV1{}, fmt.Errorf("workspace read S1 current: %w", err)
	}

	s2Checked := a.now()
	if s2Checked.IsZero() || s2Checked.Before(s1Checked) {
		return sandboxports.WorkspaceReadCurrentProjectionV1{}, errors.New("workspace read current clock regressed between S1 and S2")
	}
	if err := query.ValidateCurrent(s2Checked); err != nil {
		return sandboxports.WorkspaceReadCurrentProjectionV1{}, err
	}
	s2, err := a.readSnapshotV1(ctx, query, s2Checked)
	if err != nil {
		return sandboxports.WorkspaceReadCurrentProjectionV1{}, fmt.Errorf("workspace read S2 current: %w", err)
	}
	if !sameWorkspaceReadCurrentSnapshotV1(s1, s2) {
		return sandboxports.WorkspaceReadCurrentProjectionV1{}, errors.New("workspace read current closure drifted between S1 and S2")
	}

	expiryInputs := []int64{
		query.ExpiresUnixNano,
		s2.association.ExpiresUnixNano,
		s2.command.Meta.ExpiresUnixNano,
		s2.command.RequestedNotAfterUnixNano,
		s2.workspace.Meta.ExpiresUnixNano,
		s2.workspace.Lease.ExpiresUnixNano,
		s2.runtime.ExpiresUnixNano,
		s2.runtime.Sandbox.ExpiresUnixNano,
		s2.runtime.Sandbox.Attempt.ExpiresUnixNano,
		s2.runtime.Sandbox.RuntimeLease.Ref.ExpiresUnixNano,
		s2.runtime.Phase.ExpiresUnixNano,
		s2.runtime.Dispatch.Record.Permit.LegacyPermit.ExpiresUnixNano,
	}
	if s2.commandCurrent.Meta.ExpiresUnixNano > 0 {
		expiryInputs = append(expiryInputs, s2.commandCurrent.Meta.ExpiresUnixNano)
	}
	expires := minimumWorkspaceReadCurrentExpiryV1(expiryInputs...)
	projection := sandboxports.WorkspaceReadCurrentProjectionV1{
		QueryDigest:                      query.Digest,
		StableKeyDigest:                  query.StableKeyDigest,
		AuthorizationDigest:              query.AuthorizationDigest,
		Association:                      s2.association.Ref,
		AssociationProjectionDigest:      s2.association.ProjectionDigest,
		DomainCommand:                    s2.association.DomainCommand,
		TenantID:                         s2.command.TenantID,
		Command:                          s2.command.Meta.Ref(),
		WorkspaceView:                    s2.workspace.Meta.Ref(),
		FileScopeDigest:                  s2.command.FileScopeDigest,
		RelativePath:                     s2.command.RelativePath,
		ProviderBinding:                  s2.runtime.Sandbox.ProviderBinding,
		ReviewAuthorization:              s2.runtime.Phase.ReviewAuthorization,
		PermitID:                         s2.runtime.Phase.PermitID,
		PermitRevision:                   s2.runtime.Phase.PermitFactRevision,
		PermitDigest:                     s2.runtime.Phase.PermitDigest,
		AdmissionDigest:                  s2.runtime.Phase.AdmissionDigest,
		SandboxAttempt:                   s2.runtime.Phase.SandboxAttempt,
		SandboxProjectionRevision:        s2.runtime.Sandbox.ProjectionRevision,
		SandboxProjectionDigest:          s2.runtime.Sandbox.ProjectionDigest,
		SandboxProjectionExpiresUnixNano: s2.runtime.Sandbox.ExpiresUnixNano,
		RuntimeLease:                     s2.runtime.Sandbox.RuntimeLease,
		ExecuteEnforcement:               s2.runtime.Phase,
		RuntimeEnforcementDigest:         s2.runtime.Digest,
		S1CheckedUnixNano:                s1Checked.UnixNano(),
		S2CheckedUnixNano:                s2Checked.UnixNano(),
		ExpiresUnixNano:                  expires,
	}
	sealed, err := sandboxports.SealWorkspaceReadCurrentProjectionV1(projection)
	if err != nil {
		return sandboxports.WorkspaceReadCurrentProjectionV1{}, err
	}
	if err := sealed.ValidateCurrent(s2Checked); err != nil {
		return sandboxports.WorkspaceReadCurrentProjectionV1{}, err
	}
	return sealed, nil
}

func sameWorkspaceReadCurrentSnapshotV1(left, right workspaceReadCurrentSnapshotV1) bool {
	left.runtime.CheckedUnixNano = 0
	left.runtime.Digest = ""
	left.runtime.Dispatch.CheckedUnixNano = 0
	right.runtime.CheckedUnixNano = 0
	right.runtime.Digest = ""
	right.runtime.Dispatch.CheckedUnixNano = 0
	return reflect.DeepEqual(left, right)
}

func (a *WorkspaceReadCurrentAdapterV1) readSnapshotV1(ctx context.Context, query sandboxports.WorkspaceReadCurrentQueryV1, now time.Time) (workspaceReadCurrentSnapshotV1, error) {
	association, err := a.associations.InspectCurrentPreparedDomainCommandAssociationV1(ctx, query.Association)
	if err != nil {
		return workspaceReadCurrentSnapshotV1{}, err
	}
	if err := association.ValidateCurrent(query.Association, now); err != nil {
		return workspaceReadCurrentSnapshotV1{}, err
	}
	var command contract.WorkspaceReadCommandV1
	var commandCurrent contract.WorkspaceReadCommandOwnerCurrentV2
	if a.publishedOwner != nil {
		command, commandCurrent, err = a.publishedOwner.InspectWorkspaceReadCommandPhysicalCurrentV2(ctx, query.Command)
	} else {
		command, err = a.commands.InspectWorkspaceReadCommandCurrentV1(ctx, query.Command)
	}
	if err != nil {
		return workspaceReadCurrentSnapshotV1{}, err
	}
	if err := command.ValidateCurrent(now); err != nil {
		return workspaceReadCurrentSnapshotV1{}, err
	}
	if a.publishedOwner != nil {
		if query.PublishedCommandCurrent == nil {
			return workspaceReadCurrentSnapshotV1{}, errors.New("physical workspace read current query omitted the published Command current")
		}
		if err := commandCurrent.ValidateCurrent(now); err != nil || commandCurrent.Command != query.Command || commandCurrent.Meta.Ref() != *query.PublishedCommandCurrent {
			if err != nil {
				return workspaceReadCurrentSnapshotV1{}, err
			}
			return workspaceReadCurrentSnapshotV1{}, errors.New("published workspace read Command current drifted")
		}
	}
	workspace, err := a.workspaces.InspectWorkspaceViewCurrentV1(ctx, query.WorkspaceView)
	if err != nil {
		return workspaceReadCurrentSnapshotV1{}, err
	}
	if err := workspace.ValidateCurrent(now); err != nil {
		return workspaceReadCurrentSnapshotV1{}, err
	}
	current, err := a.runtime.InspectCurrentOperationDispatchEnforcementV4(ctx, query.RuntimeInspect)
	if err != nil {
		return workspaceReadCurrentSnapshotV1{}, err
	}
	if err := current.Validate(); err != nil {
		return workspaceReadCurrentSnapshotV1{}, err
	}
	if now.UnixNano() < current.CheckedUnixNano || !now.Before(time.Unix(0, current.ExpiresUnixNano)) {
		return workspaceReadCurrentSnapshotV1{}, errors.New("runtime enforcement current is expired or from the future")
	}
	if err := validateWorkspaceReadCurrentBindingsV1(query, association, command, workspace, current); err != nil {
		return workspaceReadCurrentSnapshotV1{}, err
	}
	if a.publishedOwner != nil {
		finalCommand, finalCommandCurrent, finalErr := a.publishedOwner.InspectWorkspaceReadCommandPhysicalCurrentV2(ctx, query.Command)
		if finalErr != nil {
			return workspaceReadCurrentSnapshotV1{}, finalErr
		}
		if !reflect.DeepEqual(finalCommand, command) || finalCommandCurrent.Meta.Ref() != commandCurrent.Meta.Ref() {
			return workspaceReadCurrentSnapshotV1{}, errors.New("published workspace read Command current drifted before snapshot return")
		}
		command = finalCommand
		commandCurrent = finalCommandCurrent
	}
	finalNow := a.now()
	if finalNow.IsZero() || finalNow.Before(now) {
		return workspaceReadCurrentSnapshotV1{}, errors.New("workspace read current clock regressed before snapshot return")
	}
	if err := query.ValidateCurrent(finalNow); err != nil {
		return workspaceReadCurrentSnapshotV1{}, err
	}
	if err := association.ValidateCurrent(query.Association, finalNow); err != nil {
		return workspaceReadCurrentSnapshotV1{}, err
	}
	if err := command.ValidateCurrent(finalNow); err != nil {
		return workspaceReadCurrentSnapshotV1{}, err
	}
	if a.publishedOwner != nil {
		if err := commandCurrent.ValidateCurrent(finalNow); err != nil {
			return workspaceReadCurrentSnapshotV1{}, err
		}
	}
	if err := workspace.ValidateCurrent(finalNow); err != nil {
		return workspaceReadCurrentSnapshotV1{}, err
	}
	if finalNow.UnixNano() < current.CheckedUnixNano || !finalNow.Before(time.Unix(0, current.ExpiresUnixNano)) {
		return workspaceReadCurrentSnapshotV1{}, errors.New("runtime enforcement current expired before snapshot return")
	}
	return workspaceReadCurrentSnapshotV1{association: association, command: command, commandCurrent: commandCurrent, workspace: workspace, runtime: current}, nil
}

func validateWorkspaceReadCurrentBindingsV1(
	query sandboxports.WorkspaceReadCurrentQueryV1,
	association runtimeports.PreparedDomainCommandAssociationCurrentProjectionV1,
	command contract.WorkspaceReadCommandV1,
	workspace contract.WorkspaceView,
	current runtimeports.CurrentOperationDispatchEnforcementV4,
) error {
	if association.Ref != query.Association ||
		query.Authorization.Association != association.Ref ||
		association.DomainCommand != query.DomainCommand ||
		query.Authorization.DomainCommand != association.DomainCommand ||
		association.DomainCommand.ID != command.Meta.ID ||
		uint64(association.DomainCommand.Revision) != command.Meta.Revision ||
		association.DomainCommand.Digest != runtimeDigestV1(command.Meta.Digest) {
		return errors.New("association, domain command, or Sandbox command exact ref drifted")
	}
	if command.Meta.Ref() != query.Command ||
		command.WorkspaceView != query.WorkspaceView ||
		workspace.Meta.Ref() != query.WorkspaceView ||
		command.FileScopeDigest != query.FileScopeDigest ||
		workspace.FileScopeDigest != query.FileScopeDigest ||
		command.RelativePath != query.RelativePath {
		return errors.New("workspace read command, view, scope, or path drifted")
	}
	if !withinWorkspaceReadScopeV1(command.RelativePath, workspace.ReadScopes) ||
		withinWorkspaceReadScopeV1(command.RelativePath, workspace.HiddenScopes) {
		return errors.New("workspace read path is outside the current visible read scope")
	}
	if association.PayloadSchema.Key() != command.SourceToolPayloadSchema ||
		string(association.PayloadDigest) != command.SourceToolPayloadDigest ||
		uint64(association.PayloadRevision) != command.SourceToolPayloadRevision {
		return errors.New("workspace read source Tool payload drifted from the Runtime association")
	}
	if association.OperationDigest != current.Sandbox.OperationDigest ||
		association.EffectID != current.Sandbox.EffectID ||
		association.EffectRevision != current.Sandbox.IntentRevision ||
		association.IntentDigest != current.Sandbox.IntentDigest ||
		association.Attempt.AttemptID != current.Sandbox.AttemptID ||
		association.Prepared.Digest != runtimeDigestV1(command.PreparedDigest) ||
		association.Provider != current.Sandbox.ProviderBinding {
		return errors.New("association drifted from Runtime enforcement or Sandbox command")
	}
	if !runtimeports.SameOperationSubjectV3(query.Authorization.Operation, current.Sandbox.Operation) ||
		query.Authorization.OperationDigest != current.Sandbox.OperationDigest ||
		!reflect.DeepEqual(query.Authorization.Attempt, association.Attempt) ||
		query.Authorization.Prepared != association.Prepared ||
		query.Authorization.Provider != current.Sandbox.ProviderBinding ||
		query.Authorization.ExecuteEnforcement != current.Phase {
		return errors.New("sealed physical authorization drifted from the exact Owner current closure")
	}
	dispatchDigest, err := runtimecore.CanonicalJSONDigest("praxis.sandbox.workspace-read", "1.0.0", "OperationDispatchAttemptRefV3", association.Attempt)
	if err != nil {
		return err
	}
	if command.OperationDigest != string(current.Sandbox.OperationDigest) ||
		command.EffectID != string(current.Sandbox.EffectID) ||
		command.IntentRevision != uint64(current.Sandbox.IntentRevision) ||
		command.IntentDigest != string(current.Sandbox.IntentDigest) ||
		command.AttemptID != current.Sandbox.AttemptID ||
		command.DispatchDigest != string(dispatchDigest) ||
		command.ProviderComponent != string(current.Sandbox.ProviderBinding.ComponentID) ||
		command.ProviderManifest != string(current.Sandbox.ProviderBinding.ManifestDigest) {
		return errors.New("workspace read command drifted from Runtime enforcement")
	}
	if query.RuntimeInspect.Inspect.Operation.Validate() != nil ||
		!runtimeports.SameOperationSubjectV3(query.RuntimeInspect.Inspect.Operation, current.Sandbox.Operation) ||
		query.RuntimeInspect.Inspect.EffectID != current.Sandbox.EffectID ||
		query.RuntimeInspect.Inspect.PermitID != current.Phase.PermitID ||
		query.RuntimeInspect.Inspect.Phase != current.Phase.Phase ||
		query.RuntimeInspect.PermitDigest != current.Phase.PermitDigest ||
		query.RuntimeInspect.AdmissionDigest != current.Phase.AdmissionDigest ||
		query.RuntimeInspect.ReviewAuthorization != current.Phase.ReviewAuthorization ||
		query.RuntimeInspect.SandboxAttempt != current.Phase.SandboxAttempt ||
		query.RuntimeInspect.SandboxProjectionDigest != current.Sandbox.ProjectionDigest {
		return errors.New("Runtime current query drifted from the independently inspected current")
	}
	tenant := string(current.Sandbox.Operation.ExecutionScope.Identity.TenantID)
	lease := current.Sandbox.RuntimeLease
	if command.TenantID != tenant ||
		workspace.Lease.TenantID != tenant ||
		workspace.Lease.InstanceID != string(lease.Instance.ID) ||
		workspace.Lease.InstanceEpoch != uint64(lease.Instance.Epoch) ||
		workspace.Lease.LeaseID != string(lease.Lease.ID) ||
		workspace.Lease.LeaseEpoch != uint64(lease.Lease.Epoch) ||
		workspace.Lease.FenceEpoch != uint64(lease.FenceEpoch) ||
		workspace.Lease.ScopeDigest != string(lease.ScopeDigest) ||
		workspace.Lease.ObservedRevision != uint64(lease.ObservedRevision) {
		return errors.New("workspace lease, fence, tenant, or scope drifted from Runtime current")
	}
	return nil
}

func withinWorkspaceReadScopeV1(value string, scopes []string) bool {
	for _, scope := range scopes {
		if value == scope || strings.HasPrefix(value, scope+"/") {
			return true
		}
	}
	return false
}

func runtimeDigestV1(value string) runtimecore.Digest {
	if strings.HasPrefix(value, "sha256:") {
		return runtimecore.Digest(value)
	}
	value = "sha256:" + value
	return runtimecore.Digest(value)
}

func minimumWorkspaceReadCurrentExpiryV1(values ...int64) int64 {
	minimum := values[0]
	for _, value := range values[1:] {
		if value < minimum {
			minimum = value
		}
	}
	return minimum
}

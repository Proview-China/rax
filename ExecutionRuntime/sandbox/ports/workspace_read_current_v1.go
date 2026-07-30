package ports

import (
	"context"
	"errors"
	"time"

	runtimecore "github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	runtimeports "github.com/Proview-China/rax/ExecutionRuntime/runtime/ports"
	"github.com/Proview-China/rax/ExecutionRuntime/sandbox/contract"
)

const WorkspaceReadCurrentContractVersionV1 = "praxis.sandbox/workspace-read-current/v1"

// WorkspaceReadCurrentQueryV1 is the Sandbox-owned, exact read coordinate
// required immediately before a bounded workspace read reaches the data plane.
// It carries no mutable current selected by the caller and grants no Provider
// authority.
type WorkspaceReadCurrentQueryV1 struct {
	ContractVersion     string                                                           `json:"contract_version"`
	RuntimeInspect      runtimeports.InspectCurrentOperationDispatchEnforcementRequestV4 `json:"runtime_inspect"`
	Authorization       runtimeports.ControlledOperationPhysicalExecutionAuthorizationV3 `json:"authorization"`
	StableKeyDigest     runtimecore.Digest                                               `json:"stable_key_digest"`
	AuthorizationDigest runtimecore.Digest                                               `json:"authorization_digest"`
	Association         runtimeports.PreparedDomainCommandAssociationRefV1               `json:"association"`
	DomainCommand       runtimeports.OperationDomainCommandRefV1                         `json:"domain_command"`
	Command             contract.Ref                                                     `json:"command"`
	WorkspaceView       contract.Ref                                                     `json:"workspace_view"`
	FileScopeDigest     string                                                           `json:"file_scope_digest"`
	RelativePath        string                                                           `json:"relative_path"`
	CheckedUnixNano     int64                                                            `json:"checked_unix_nano"`
	ExpiresUnixNano     int64                                                            `json:"expires_unix_nano"`
	Digest              string                                                           `json:"digest"`
}

func (q WorkspaceReadCurrentQueryV1) Validate() error {
	if q.ContractVersion != WorkspaceReadCurrentContractVersionV1 ||
		q.RuntimeInspect.Validate() != nil ||
		q.Authorization.Validate() != nil ||
		q.StableKeyDigest.Validate() != nil ||
		q.AuthorizationDigest.Validate() != nil ||
		q.Association.Validate() != nil ||
		q.DomainCommand.Validate() != nil ||
		q.Command.ValidateShape("workspace read command") != nil ||
		q.WorkspaceView.ValidateShape("workspace view") != nil ||
		!contract.ValidDigest(q.FileScopeDigest) ||
		contract.ValidateLogicalPath(q.RelativePath) != nil ||
		q.CheckedUnixNano <= 0 ||
		q.ExpiresUnixNano <= q.CheckedUnixNano ||
		!contract.ValidDigest(q.Digest) {
		return errors.New("workspace read current query is incomplete")
	}
	if q.StableKeyDigest != q.Authorization.StableKeyDigest ||
		q.AuthorizationDigest != q.Authorization.AuthorizationDigest ||
		q.Association != q.Authorization.Association ||
		q.DomainCommand != q.Authorization.DomainCommand ||
		q.ExpiresUnixNano > q.Authorization.UnifiedNotAfterUnixNano {
		return errors.New("workspace read current query drifted from the sealed physical authorization")
	}
	copy := q
	copy.Digest = ""
	digest, err := contract.Digest("workspace-read-current-query", copy)
	if err != nil || digest != q.Digest {
		return errors.New("workspace read current query digest drifted")
	}
	return nil
}

func (q WorkspaceReadCurrentQueryV1) ValidateCurrent(now time.Time) error {
	if err := q.Validate(); err != nil {
		return err
	}
	if err := q.Authorization.ValidateCurrent(now); err != nil {
		return err
	}
	if now.IsZero() || now.UnixNano() < q.CheckedUnixNano {
		return errors.New("workspace read current query clock regressed")
	}
	if !now.Before(time.Unix(0, q.ExpiresUnixNano)) {
		return errors.New("workspace read current query expired")
	}
	return nil
}

func SealWorkspaceReadCurrentQueryV1(q WorkspaceReadCurrentQueryV1) (WorkspaceReadCurrentQueryV1, error) {
	q.ContractVersion = WorkspaceReadCurrentContractVersionV1
	q.Digest = ""
	digest, err := contract.Digest("workspace-read-current-query", q)
	if err != nil {
		return WorkspaceReadCurrentQueryV1{}, err
	}
	q.Digest = digest
	return q, q.Validate()
}

// WorkspaceReadCurrentProjectionV1 is a read-only aggregate. Runtime remains
// the owner of enforcement, Review and Permit facts; Sandbox remains the owner
// of command, workspace and physical-read facts.
type WorkspaceReadCurrentProjectionV1 struct {
	ContractVersion                  string                                              `json:"contract_version"`
	QueryDigest                      string                                              `json:"query_digest"`
	StableKeyDigest                  runtimecore.Digest                                  `json:"stable_key_digest"`
	AuthorizationDigest              runtimecore.Digest                                  `json:"authorization_digest"`
	Association                      runtimeports.PreparedDomainCommandAssociationRefV1  `json:"association"`
	AssociationProjectionDigest      runtimecore.Digest                                  `json:"association_projection_digest"`
	DomainCommand                    runtimeports.OperationDomainCommandRefV1            `json:"domain_command"`
	TenantID                         string                                              `json:"tenant_id"`
	Command                          contract.Ref                                        `json:"command"`
	WorkspaceView                    contract.Ref                                        `json:"workspace_view"`
	FileScopeDigest                  string                                              `json:"file_scope_digest"`
	RelativePath                     string                                              `json:"relative_path"`
	ProviderBinding                  runtimeports.ProviderBindingRefV2                   `json:"provider_binding"`
	ReviewAuthorization              runtimeports.OperationReviewAuthorizationRefV4      `json:"review_authorization"`
	PermitID                         string                                              `json:"permit_id"`
	PermitRevision                   runtimecore.Revision                                `json:"permit_revision"`
	PermitDigest                     runtimecore.Digest                                  `json:"permit_digest"`
	AdmissionDigest                  runtimecore.Digest                                  `json:"admission_digest"`
	SandboxAttempt                   runtimeports.OperationDispatchSandboxFactRefV4      `json:"sandbox_attempt"`
	SandboxProjectionRevision        runtimecore.Revision                                `json:"sandbox_projection_revision"`
	SandboxProjectionDigest          runtimecore.Digest                                  `json:"sandbox_projection_digest"`
	SandboxProjectionExpiresUnixNano int64                                               `json:"sandbox_projection_expires_unix_nano"`
	RuntimeLease                     runtimeports.OperationDispatchRuntimeLeaseBindingV4 `json:"runtime_lease"`
	ExecuteEnforcement               runtimeports.OperationDispatchEnforcementPhaseRefV4 `json:"execute_enforcement"`
	RuntimeEnforcementDigest         runtimecore.Digest                                  `json:"runtime_enforcement_digest"`
	S1CheckedUnixNano                int64                                               `json:"s1_checked_unix_nano"`
	S2CheckedUnixNano                int64                                               `json:"s2_checked_unix_nano"`
	ExpiresUnixNano                  int64                                               `json:"expires_unix_nano"`
	SemanticDigest                   string                                              `json:"semantic_digest"`
	ProjectionDigest                 string                                              `json:"projection_digest"`
}

func (p WorkspaceReadCurrentProjectionV1) ValidateCurrent(now time.Time) error {
	if p.ContractVersion != WorkspaceReadCurrentContractVersionV1 ||
		!contract.ValidDigest(p.QueryDigest) ||
		p.StableKeyDigest.Validate() != nil ||
		p.AuthorizationDigest.Validate() != nil ||
		p.Association.Validate() != nil ||
		p.AssociationProjectionDigest.Validate() != nil ||
		p.DomainCommand.Validate() != nil ||
		p.TenantID == "" ||
		p.Command.ValidateShape("workspace read command") != nil ||
		p.WorkspaceView.ValidateShape("workspace view") != nil ||
		!contract.ValidDigest(p.FileScopeDigest) ||
		contract.ValidateLogicalPath(p.RelativePath) != nil ||
		p.ProviderBinding.Validate() != nil ||
		p.ReviewAuthorization.Validate() != nil ||
		p.PermitID == "" ||
		p.PermitRevision == 0 ||
		p.PermitDigest.Validate() != nil ||
		p.AdmissionDigest.Validate() != nil ||
		p.SandboxAttempt.Validate() != nil ||
		p.SandboxProjectionRevision == 0 ||
		p.SandboxProjectionDigest.Validate() != nil ||
		p.SandboxProjectionExpiresUnixNano <= 0 ||
		p.RuntimeLease.Validate() != nil ||
		p.ExecuteEnforcement.Validate() != nil ||
		p.RuntimeEnforcementDigest.Validate() != nil ||
		p.S1CheckedUnixNano <= 0 ||
		p.S2CheckedUnixNano < p.S1CheckedUnixNano ||
		p.ExpiresUnixNano <= p.S2CheckedUnixNano ||
		p.ExpiresUnixNano > p.SandboxProjectionExpiresUnixNano ||
		!contract.ValidDigest(p.SemanticDigest) ||
		!contract.ValidDigest(p.ProjectionDigest) {
		return errors.New("workspace read current projection is incomplete")
	}
	semantic, err := workspaceReadCurrentSemanticDigestV1(p)
	if err != nil || semantic != p.SemanticDigest {
		return errors.New("workspace read current semantic digest drifted")
	}
	copy := p
	copy.ProjectionDigest = ""
	digest, err := contract.Digest("workspace-read-current-projection", copy)
	if err != nil || digest != p.ProjectionDigest {
		return errors.New("workspace read current projection digest drifted")
	}
	if now.IsZero() || now.UnixNano() < p.S2CheckedUnixNano {
		return errors.New("workspace read current projection clock regressed")
	}
	if !now.Before(time.Unix(0, p.ExpiresUnixNano)) {
		return errors.New("workspace read current projection expired")
	}
	return nil
}

func SealWorkspaceReadCurrentProjectionV1(p WorkspaceReadCurrentProjectionV1) (WorkspaceReadCurrentProjectionV1, error) {
	p.ContractVersion = WorkspaceReadCurrentContractVersionV1
	p.SemanticDigest = ""
	p.ProjectionDigest = ""
	semantic, err := workspaceReadCurrentSemanticDigestV1(p)
	if err != nil {
		return WorkspaceReadCurrentProjectionV1{}, err
	}
	p.SemanticDigest = semantic
	digest, err := contract.Digest("workspace-read-current-projection", p)
	if err != nil {
		return WorkspaceReadCurrentProjectionV1{}, err
	}
	p.ProjectionDigest = digest
	return p, nil
}

// SemanticDigest identifies the unchanged exact-current closure. The outer
// ProjectionDigest still seals the full response, including fresh read times
// and Runtime's fresh aggregate envelope digest.
func workspaceReadCurrentSemanticDigestV1(p WorkspaceReadCurrentProjectionV1) (string, error) {
	p.ProjectionDigest = ""
	p.SemanticDigest = ""
	p.S1CheckedUnixNano = 0
	p.S2CheckedUnixNano = 0
	p.RuntimeEnforcementDigest = ""
	return contract.Digest("workspace-read-current-projection", p)
}

// OperationDispatchEnforcementCurrentReaderV4 is deliberately narrow. It
// structurally consumes Runtime's exact current reader without importing a
// broad Runtime governance/write surface.
type OperationDispatchEnforcementCurrentReaderV4 interface {
	InspectCurrentOperationDispatchEnforcementV4(context.Context, runtimeports.InspectCurrentOperationDispatchEnforcementRequestV4) (runtimeports.CurrentOperationDispatchEnforcementV4, error)
}

type WorkspaceReadCurrentProjectionReaderV1 interface {
	InspectWorkspaceReadCurrentV1(context.Context, WorkspaceReadCurrentQueryV1) (WorkspaceReadCurrentProjectionV1, error)
}

package applicationadapter

import (
	"errors"
	"io/fs"
	"path/filepath"
	"time"

	applicationports "github.com/Proview-China/rax/ExecutionRuntime/application/ports"
	runtimeports "github.com/Proview-China/rax/ExecutionRuntime/runtime/ports"
	"github.com/Proview-China/rax/ExecutionRuntime/sandbox/dataplaneadapter"
	"github.com/Proview-China/rax/ExecutionRuntime/sandbox/kernel"
	sandboxports "github.com/Proview-China/rax/ExecutionRuntime/sandbox/ports"
	"github.com/Proview-China/rax/ExecutionRuntime/sandbox/runtimeadapter"
)

// ProductionCompositionConfigV4 contains only public Owner ports and durable
// Sandbox stores. It deliberately does not construct a Runtime gateway or a
// State Plane implementation and never accepts a Provider implementation.
type ProductionCompositionConfigV4 struct {
	Facts                 sandboxports.FactStore
	Current               sandboxports.ExactCurrentStore
	GenerationBindings    runtimeports.GenerationBindingAssociationGovernancePortV1
	Enforcement           runtimeports.OperationDispatchEnforcementGovernancePortV4
	CheckpointEnforcement runtimeports.CheckpointRestoreDispatchEnforcementGovernancePortV1
	Evidence              runtimeports.OperationScopeEvidenceGovernancePortV3
	Settlements           runtimeports.OperationSettlementGovernancePortV4
	DomainResultBindings  runtimeadapter.DomainResultRuntimeBindingStoreV4
	LifecyclePlans        LifecyclePlanReaderV4
	LifecycleResults      LifecycleApplicationResultStoreV4
	Workspace             sandboxports.WorkspaceOwnerStoreV1
	DomainResultOwner     runtimeports.ProviderBindingRefV2
	DomainResultSchema    runtimeports.SchemaRefV2

	DataPlaneSocketPath string
	DataPlaneAllowedUID uint32
	CurrentSocketPath   string
	CurrentSocketMode   fs.FileMode
	CurrentAllowedUID   uint32
	Now                 func() time.Time
}

// ProductionCompositionV4 is the Sandbox-owned assembly contribution. The
// host must inject its Runtime Owner gateways and durable State Plane stores.
// SandboxCurrent is wired into Runtime's V4 gateway by the host; CurrentServer
// is the reverse-current endpoint used by the Rust actual execution point.
type ProductionCompositionV4 struct {
	Lifecycle      applicationports.SandboxLifecyclePortV4
	SandboxCurrent runtimeports.OperationDispatchSandboxCurrentReaderV4
	CurrentServer  dataplaneadapter.CurrentServer
}

func NewProductionCompositionV4(config ProductionCompositionConfigV4) (*ProductionCompositionV4, error) {
	if nilLike(config.Facts) || nilLike(config.Current) || nilLike(config.GenerationBindings) ||
		nilLike(config.Enforcement) || nilLike(config.CheckpointEnforcement) || nilLike(config.Evidence) || nilLike(config.Settlements) ||
		nilLike(config.DomainResultBindings) || nilLike(config.LifecyclePlans) || nilLike(config.LifecycleResults) || nilLike(config.Now) {
		return nil, errors.New("production Sandbox composition requires all Owner ports, durable stores, and a clock")
	}
	if config.DomainResultOwner.Validate() != nil || config.DomainResultSchema.Validate() != nil {
		return nil, errors.New("production Sandbox composition requires exact DomainResult owner and schema refs")
	}
	if config.DataPlaneSocketPath == "" || config.CurrentSocketPath == "" ||
		!filepath.IsAbs(config.DataPlaneSocketPath) || !filepath.IsAbs(config.CurrentSocketPath) ||
		filepath.Clean(config.DataPlaneSocketPath) == filepath.Clean(config.CurrentSocketPath) ||
		config.CurrentSocketMode == 0 || config.CurrentSocketMode&0o007 != 0 || config.CurrentSocketMode&^fs.FileMode(0o777) != 0 {
		return nil, errors.New("production Sandbox composition requires distinct absolute, closed UDS paths and mode")
	}

	controller, err := kernel.NewController(config.Facts, config.Now)
	if err != nil {
		return nil, err
	}
	currentReader, err := runtimeadapter.NewCurrentReaderV4(config.Current, config.GenerationBindings, config.Now)
	if err != nil {
		return nil, err
	}
	domain, err := runtimeadapter.NewDomainResultCurrentAdapterV4(config.Facts, config.DomainResultBindings, config.DomainResultOwner, config.DomainResultSchema, config.Now)
	if err != nil {
		return nil, err
	}
	dataPlane := dataplaneadapter.Client{SocketPath: config.DataPlaneSocketPath, AllowedUID: config.DataPlaneAllowedUID}
	boundary, err := NewProviderBoundaryV4(config.Enforcement, config.Evidence, dataPlane, config.Now)
	if err != nil {
		return nil, err
	}
	inspector, err := NewGovernedInspectionV4(controller, config.Facts, boundary, domain, config.Settlements, config.Now)
	if err != nil {
		return nil, err
	}
	flow, err := NewLifecycleFlowV4(controller, config.Facts, boundary, inspector, domain, config.Settlements, config.Now)
	if err != nil {
		return nil, err
	}
	lifecycle, err := NewApplicationLifecycleV4(flow, config.LifecyclePlans, config.LifecycleResults, config.Now)
	if err != nil {
		return nil, err
	}
	if !nilLike(config.Workspace) {
		workspace, workspaceErr := NewWorkspaceCommitFlowV1(flow, config.Workspace)
		if workspaceErr != nil {
			return nil, workspaceErr
		}
		lifecycle, err = lifecycle.WithWorkspaceCommitV1(workspace)
		if err != nil {
			return nil, err
		}
	}

	return &ProductionCompositionV4{
		Lifecycle:      lifecycle,
		SandboxCurrent: currentReader,
		CurrentServer: dataplaneadapter.CurrentServer{
			SocketPath:           config.CurrentSocketPath,
			SocketMode:           config.CurrentSocketMode,
			AllowedUID:           config.CurrentAllowedUID,
			Governance:           config.Enforcement,
			CheckpointGovernance: config.CheckpointEnforcement,
			Sandbox:              config.Current,
			Now:                  config.Now,
		},
	}, nil
}

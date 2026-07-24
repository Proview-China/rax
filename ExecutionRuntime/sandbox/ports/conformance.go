package ports

import (
	"context"
	"errors"
	"fmt"

	"github.com/Proview-China/rax/ExecutionRuntime/sandbox/contract"
)

var ErrUnsupported = errors.New("sandbox capability is not implemented in this build")

type BackendConformanceRequest struct {
	Backend     contract.BackendDescriptor
	Requirement contract.ExecutionRequirement
}

// BackendConformancePort evaluates pure capability evidence. It cannot execute,
// allocate, activate, open, inspect a remote provider, or certify production SLA.
type BackendConformancePort interface {
	Assess(context.Context, BackendConformanceRequest) (contract.BackendConformanceReport, error)
}

type Feature string

const (
	FeaturePureDomainModel         Feature = "pure_domain_model"
	FeatureLocalConformance        Feature = "local_conformance"
	FeatureExactCurrentReader      Feature = "exact_current_reader_v4"
	FeatureCheckpointParticipant   Feature = "checkpoint_participant_v2"
	FeatureSnapshotArtifactCapture Feature = "snapshot_artifact_capture_v2"
	FeatureSnapshotArtifactOwner   Feature = "snapshot_artifact_owner_v2"
	FeatureExternalLifecycle       Feature = "external_lifecycle"
	FeatureRuntimeAdapter          Feature = "runtime_adapter"
	FeatureApplicationAdapter      Feature = "application_adapter"
	FeatureCheckpointRestore       Feature = "checkpoint_restore"
	FeatureAssemblyBinding         Feature = "assembly_binding"
	FeatureOwnerSQLite             Feature = "owner_sqlite_state_plane"
	FeatureHostWorkspaceBackend    Feature = "host_workspace_backend"
	FeatureContainerBackend        Feature = "container_backend"
	FeatureMicroVMBackend          Feature = "microvm_backend"
	FeatureWASMCapability          Feature = "wasm_capability_backend"
	FeatureWorkspaceCapture        Feature = "workspace_overlay_capture"
	FeatureWorkspaceCommit         Feature = "workspace_governed_commit"
	FeatureRemoteBackend           Feature = "remote_backend"
	FeatureGovernedAPI             Feature = "governed_sdk_cli_api"
)

func Supported(feature Feature) bool {
	switch feature {
	case FeaturePureDomainModel, FeatureLocalConformance, FeatureExactCurrentReader,
		FeatureOwnerSQLite, FeatureHostWorkspaceBackend, FeatureContainerBackend,
		FeatureMicroVMBackend, FeatureWASMCapability, FeatureWorkspaceCapture,
		FeatureCheckpointParticipant, FeatureExternalLifecycle, FeatureRuntimeAdapter,
		FeatureApplicationAdapter, FeatureCheckpointRestore, FeatureAssemblyBinding,
		FeatureWorkspaceCommit, FeatureRemoteBackend, FeatureGovernedAPI,
		FeatureSnapshotArtifactCapture:
		return true
	default:
		return false
	}
}

func RequireSupported(feature Feature) error {
	if Supported(feature) {
		return nil
	}
	return fmt.Errorf("%w: %s", ErrUnsupported, feature)
}

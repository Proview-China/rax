use std::collections::{BTreeMap, BTreeSet};
use std::time::{SystemTime, UNIX_EPOCH};

use serde::{Deserialize, Serialize};
use sha2::{Digest as _, Sha256};

use crate::error::{ClosedError, ClosedReason, Result};

pub const CONTRACT_VERSION_V1: &str = "praxis.sandbox/data-plane-ipc/v1";
pub const MAX_FRAME_BYTES: usize = 4 * 1024 * 1024;

#[derive(Clone, Debug, Eq, PartialEq, Serialize, Deserialize)]
#[serde(deny_unknown_fields)]
pub struct ExactRefV1 {
    pub id: String,
    pub revision: u64,
    pub digest: String,
    pub expires_unix_nano: i64,
}

#[derive(Clone, Debug, Eq, PartialEq, Serialize, Deserialize)]
#[serde(deny_unknown_fields)]
pub struct ProviderBindingV1 {
    pub binding_set_id: String,
    pub binding_set_revision: u64,
    pub component_id: String,
    pub manifest_digest: String,
    pub artifact_digest: String,
    pub capability: String,
    pub digest: String,
}

impl ProviderBindingV1 {
    pub fn validate(&self) -> Result<()> {
        if self.binding_set_id.trim().is_empty()
            || self.binding_set_revision == 0
            || self.component_id.trim().is_empty()
            || self.capability.trim().is_empty()
            || !valid_digest(&self.manifest_digest)
            || !valid_digest(&self.artifact_digest)
            || self.digest != self.calculate_digest()?
        {
            return Err(ClosedError::new(
                ClosedReason::InvalidContract,
                "provider binding is incomplete or drifted",
            ));
        }
        Ok(())
    }

    pub fn calculate_digest(&self) -> Result<String> {
        let mut canonical = self.clone();
        canonical.digest.clear();
        canonical_digest("ProviderBindingV1", &canonical)
    }
}

#[derive(Clone, Debug, Eq, PartialEq, Serialize, Deserialize)]
#[serde(deny_unknown_fields)]
pub struct SandboxProjectionRefV1 {
    pub revision: u64,
    pub digest: String,
    pub expires_unix_nano: i64,
}

impl SandboxProjectionRefV1 {
    pub fn validate_current(&self, now_unix_nano: i64) -> Result<()> {
        if self.revision == 0
            || !valid_digest(&self.digest)
            || self.expires_unix_nano <= 0
            || now_unix_nano <= 0
            || now_unix_nano >= self.expires_unix_nano
        {
            return Err(ClosedError::new(
                ClosedReason::CurrentExpired,
                "sandbox projection ref is incomplete or expired",
            ));
        }
        Ok(())
    }
}

#[derive(Clone, Debug, Eq, PartialEq, Serialize, Deserialize)]
#[serde(deny_unknown_fields)]
pub struct RuntimeEnforcementRefV1 {
    pub operation_digest: String,
    pub effect_id: String,
    pub permit_id: String,
    pub attempt_id: String,
    pub phase: EnforcementPhaseV1,
    pub receipt_digest: String,
    pub journal_revision: u64,
    pub expires_unix_nano: i64,
}

#[derive(Clone, Debug, Eq, PartialEq, Serialize, Deserialize)]
#[serde(deny_unknown_fields)]
pub struct ExecutionBindingV1 {
    pub tenant_id: String,
    pub instance_id: String,
    pub instance_epoch: u64,
    pub lease_id: String,
    pub lease_epoch: u64,
    pub fence_epoch: u64,
    pub scope_digest: String,
    pub observed_revision: u64,
    pub expires_unix_nano: i64,
}

impl ExecutionBindingV1 {
    pub fn validate(&self) -> Result<()> {
        if self.tenant_id.trim().is_empty()
            || self.instance_id.trim().is_empty()
            || self.lease_id.trim().is_empty()
            || self.instance_epoch == 0
            || self.lease_epoch == 0
            || self.fence_epoch == 0
            || self.observed_revision == 0
            || !valid_digest(&self.scope_digest)
            || self.expires_unix_nano <= 0
        {
            return Err(ClosedError::new(
                ClosedReason::InvalidContract,
                "execution binding is incomplete",
            ));
        }
        Ok(())
    }

    pub fn validate_current(&self, now_unix_nano: i64) -> Result<()> {
        self.validate()?;
        if now_unix_nano <= 0 || now_unix_nano >= self.expires_unix_nano {
            return Err(ClosedError::new(
                ClosedReason::CurrentExpired,
                "execution binding is expired",
            ));
        }
        Ok(())
    }
}

impl RuntimeEnforcementRefV1 {
    pub fn validate(&self) -> Result<()> {
        if !valid_digest(&self.operation_digest)
            || self.effect_id.trim().is_empty()
            || self.permit_id.trim().is_empty()
            || self.attempt_id.trim().is_empty()
            || !valid_digest(&self.receipt_digest)
            || self.journal_revision == 0
            || self.expires_unix_nano <= 0
        {
            return Err(ClosedError::new(
                ClosedReason::InvalidContract,
                "runtime enforcement ref is incomplete",
            ));
        }
        Ok(())
    }

    pub fn validate_current(&self, now_unix_nano: i64) -> Result<()> {
        self.validate()?;
        if now_unix_nano <= 0 || now_unix_nano >= self.expires_unix_nano {
            return Err(ClosedError::new(
                ClosedReason::CurrentExpired,
                "runtime enforcement ref is expired",
            ));
        }
        Ok(())
    }
}

impl ExactRefV1 {
    pub fn validate(&self, name: &str) -> Result<()> {
        if self.id.trim().is_empty()
            || self.revision == 0
            || !valid_digest(&self.digest)
            || self.expires_unix_nano <= 0
        {
            return Err(ClosedError::new(
                ClosedReason::InvalidArgument,
                format!("{name} exact reference is incomplete"),
            ));
        }
        Ok(())
    }

    pub fn validate_current(&self, name: &str, now_unix_nano: i64) -> Result<()> {
        self.validate(name)?;
        if now_unix_nano <= 0 || now_unix_nano >= self.expires_unix_nano {
            return Err(ClosedError::new(
                ClosedReason::CurrentExpired,
                format!("{name} exact reference is expired"),
            ));
        }
        Ok(())
    }
}

#[derive(Clone, Copy, Debug, Eq, PartialEq, Serialize, Deserialize)]
#[serde(rename_all = "snake_case")]
pub enum EnforcementPhaseV1 {
    Prepare,
    Execute,
}

#[derive(Clone, Copy, Debug, Eq, PartialEq, Serialize, Deserialize)]
#[serde(rename_all = "snake_case")]
pub enum DataPlaneOperationV1 {
    Dispatch,
    Inspect,
}

#[derive(Clone, Debug, Eq, PartialEq, Serialize, Deserialize)]
#[serde(deny_unknown_fields)]
pub struct DataPlaneRequestV1 {
    pub contract_version: String,
    pub operation: DataPlaneOperationV1,
    pub request: DispatchRequestV1,
}

impl DataPlaneRequestV1 {
    pub fn validate(&self, now: i64) -> Result<()> {
        if self.contract_version != CONTRACT_VERSION_V1 {
            return Err(ClosedError::new(
                ClosedReason::InvalidContract,
                "data plane request contract version is invalid",
            ));
        }
        self.request.validate_current(now)
    }
}

#[derive(Clone, Copy, Debug, Eq, PartialEq, Serialize, Deserialize)]
#[serde(rename_all = "snake_case")]
pub enum ProviderKindV1 {
    HostWorkspace,
    QemuMicrovm,
    ContainerdOci,
    WasmtimeComponent,
    RemoteSandbox,
    WorkspaceCommit,
}

#[derive(Clone, Debug, Eq, PartialEq, Serialize, Deserialize)]
#[serde(deny_unknown_fields)]
pub struct ProviderInspectionTargetV1 {
    pub original_effect_kind: String,
    pub original_attempt_id: String,
    pub provider_attempt: ExactRefV1,
    pub original_request_digest: String,
    pub original_payload_digest: String,
}

impl ProviderInspectionTargetV1 {
    fn validate(&self, provider: ProviderKindV1, tenant_id: &str, now: i64) -> Result<()> {
        if !inspectable_original_effect_kind(&self.original_effect_kind)
            || self.original_attempt_id.trim().is_empty()
            || self.provider_attempt.revision != 2
            || !valid_digest(&self.original_request_digest)
            || !valid_digest(&self.original_payload_digest)
        {
            return Err(ClosedError::new(
                ClosedReason::InvalidContract,
                "inspection target coordinates are incomplete",
            ));
        }
        self.provider_attempt
            .validate_current("inspection provider attempt", now)?;
        let provider_name = match provider {
            ProviderKindV1::HostWorkspace => "host",
            ProviderKindV1::QemuMicrovm => "microvm",
            ProviderKindV1::ContainerdOci => "containerd",
            ProviderKindV1::WasmtimeComponent => "wasmtime",
            ProviderKindV1::RemoteSandbox => "remote",
            ProviderKindV1::WorkspaceCommit => "workspace-commit",
        };
        if self.provider_attempt.id
            != format!("{provider_name}/{tenant_id}/{}", self.original_attempt_id)
        {
            return Err(ClosedError::new(
                ClosedReason::BindingDrift,
                "inspection target provider attempt identity drifted",
            ));
        }
        Ok(())
    }
}

#[derive(Clone, Debug, Eq, PartialEq, Serialize, Deserialize)]
#[serde(deny_unknown_fields)]
pub struct ContainerPayloadV1 {
    pub rootfs_binding_id: String,
    pub image_digest: String,
    pub argv: Vec<String>,
    pub environment: BTreeMap<String, String>,
    pub working_directory: String,
    pub read_only_rootfs: bool,
    pub cpu_quota_micros: u64,
    pub memory_limit_bytes: u64,
    pub pids_limit: u64,
    pub network_deny_all: bool,
    #[serde(default, skip_serializing_if = "Option::is_none")]
    pub inspection_target: Option<ProviderInspectionTargetV1>,
}

impl ContainerPayloadV1 {
    fn validate(&self) -> Result<()> {
        if self.rootfs_binding_id.trim().is_empty()
            || !valid_digest(&self.image_digest)
            || self.argv.is_empty()
            || self
                .argv
                .iter()
                .any(|value| value.is_empty() || value.contains('\0'))
            || !self.working_directory.starts_with('/')
            || self.cpu_quota_micros == 0
            || self.memory_limit_bytes < 16 * 1024 * 1024
            || self.pids_limit == 0
            || !self.network_deny_all
        {
            return Err(ClosedError::new(
                ClosedReason::InvalidArgument,
                "container payload violates the first-cut closed policy",
            ));
        }
        if self.environment.iter().any(|(key, value)| {
            key.is_empty() || key.contains('=') || key.contains('\0') || value.contains('\0')
        }) {
            return Err(ClosedError::new(
                ClosedReason::InvalidArgument,
                "container environment is malformed",
            ));
        }
        Ok(())
    }
}

#[derive(Clone, Debug, Eq, PartialEq, Serialize, Deserialize)]
#[serde(deny_unknown_fields)]
pub struct WasmPayloadV1 {
    pub component_path_binding_id: String,
    pub component_digest: String,
    pub world: String,
    pub export: String,
    pub fuel: u64,
    pub epoch_deadline_ticks: u64,
    pub memory_limit_bytes: usize,
    pub table_elements_limit: usize,
    pub instance_limit: usize,
    #[serde(default, skip_serializing_if = "Vec::is_empty")]
    pub capability_bindings: Vec<WasmCapabilityBindingV1>,
    #[serde(default, skip_serializing_if = "Option::is_none")]
    pub inspection_target: Option<ProviderInspectionTargetV1>,
}

#[derive(Clone, Debug, Eq, PartialEq, Serialize, Deserialize)]
#[serde(deny_unknown_fields)]
pub struct WasmCapabilityBindingV1 {
    pub name: String,
    pub grant: ExactRefV1,
    pub request_schema: String,
    pub response_schema: String,
    pub max_request_bytes: u64,
    pub max_response_bytes: u64,
}

impl WasmCapabilityBindingV1 {
    fn validate(&self) -> Result<()> {
        if self.name.trim().is_empty()
            || self.grant.validate("WASM capability grant").is_err()
            || self.request_schema.trim().is_empty()
            || self.response_schema.trim().is_empty()
            || self.max_request_bytes == 0
            || self.max_response_bytes == 0
            || self.max_request_bytes > MAX_FRAME_BYTES as u64
            || self.max_response_bytes > MAX_FRAME_BYTES as u64
        {
            return Err(ClosedError::new(
                ClosedReason::InvalidArgument,
                "WASM capability binding violates the closed policy",
            ));
        }
        Ok(())
    }
}

impl WasmPayloadV1 {
    fn validate(&self) -> Result<()> {
        if self.component_path_binding_id.trim().is_empty()
            || !valid_digest(&self.component_digest)
            || self.world != "praxis:sandbox/capability@1.0.0"
            || self.export.trim().is_empty()
            || self.fuel == 0
            || self.epoch_deadline_ticks == 0
            || self.memory_limit_bytes == 0
            || self.table_elements_limit == 0
            || self.instance_limit == 0
        {
            return Err(ClosedError::new(
                ClosedReason::InvalidArgument,
                "WASM payload violates the first-cut closed policy",
            ));
        }
        if self.capability_bindings.len() > 64 {
            return Err(ClosedError::new(
                ClosedReason::ResourceLimit,
                "WASM capability binding limit exceeded",
            ));
        }
        let mut names = BTreeSet::new();
        for binding in &self.capability_bindings {
            binding.validate()?;
            if !names.insert(binding.name.as_str()) {
                return Err(ClosedError::new(
                    ClosedReason::Conflict,
                    "WASM capability binding is duplicated",
                ));
            }
        }
        Ok(())
    }
}

#[derive(Clone, Debug, Eq, PartialEq, Serialize, Deserialize)]
#[serde(deny_unknown_fields)]
pub struct HostWorkspacePayloadV1 {
    pub workspace_binding_id: String,
    pub workspace_digest: String,
    pub tool_binding_id: String,
    pub tool_digest: String,
    pub argv: Vec<String>,
    pub environment: BTreeMap<String, String>,
    pub working_directory: String,
    pub network_deny_all: bool,
    pub wall_clock_timeout_millis: u64,
    #[serde(default, skip_serializing_if = "Option::is_none")]
    pub inspection_target: Option<ProviderInspectionTargetV1>,
}

impl HostWorkspacePayloadV1 {
    fn validate(&self) -> Result<()> {
        let invalid_working_directory = self.working_directory.is_empty()
            || self.working_directory.starts_with('/')
            || self
                .working_directory
                .split('/')
                .any(|part| part.is_empty() || part == "." || part == "..");
        if self.workspace_binding_id.trim().is_empty()
            || !valid_digest(&self.workspace_digest)
            || self.tool_binding_id.trim().is_empty()
            || !valid_digest(&self.tool_digest)
            || self.argv.len() > 256
            || self
                .argv
                .iter()
                .any(|value| value.is_empty() || value.contains('\0'))
            || invalid_working_directory
            || !self.network_deny_all
            || self.wall_clock_timeout_millis == 0
            || self.wall_clock_timeout_millis > 86_400_000
        {
            return Err(ClosedError::new(
                ClosedReason::InvalidArgument,
                "host workspace payload violates the closed policy",
            ));
        }
        if self.environment.len() > 128
            || self.environment.iter().any(|(key, value)| {
                key.is_empty()
                    || key.contains('=')
                    || key.contains('\0')
                    || value.contains('\0')
                    || key.len() > 128
                    || value.len() > 16 * 1024
            })
        {
            return Err(ClosedError::new(
                ClosedReason::InvalidArgument,
                "host workspace environment is malformed",
            ));
        }
        Ok(())
    }
}

#[derive(Clone, Debug, Eq, PartialEq, Serialize, Deserialize)]
#[serde(deny_unknown_fields)]
pub struct MicroVmPayloadV1 {
    pub kernel_binding_id: String,
    pub kernel_digest: String,
    pub initramfs_binding_id: String,
    pub initramfs_digest: String,
    pub vcpus: u16,
    pub memory_mib: u32,
    pub network_deny_all: bool,
    pub wall_clock_timeout_millis: u64,
    #[serde(default, skip_serializing_if = "Option::is_none")]
    pub inspection_target: Option<ProviderInspectionTargetV1>,
}

#[derive(Clone, Debug, Eq, PartialEq, Serialize, Deserialize)]
#[serde(deny_unknown_fields)]
pub struct RemotePayloadV1 {
    pub endpoint_binding_id: String,
    pub endpoint_digest: String,
    pub workload_id: String,
    pub workload_digest: String,
    pub credential: ExactRefV1,
    pub isolation_profile: String,
    #[serde(default, skip_serializing_if = "Option::is_none")]
    pub inspection_target: Option<ProviderInspectionTargetV1>,
}

#[derive(Clone, Debug, Eq, PartialEq, Serialize, Deserialize)]
#[serde(deny_unknown_fields)]
pub struct WorkspaceMutationV1 {
    pub kind: String,
    pub path: String,
    #[serde(default, skip_serializing_if = "String::is_empty")]
    pub target_path: String,
    #[serde(default, skip_serializing_if = "String::is_empty")]
    pub blob_id: String,
    #[serde(default, skip_serializing_if = "String::is_empty")]
    pub blob_digest: String,
    #[serde(default, skip_serializing_if = "is_zero_u32")]
    pub mode: u32,
}

#[derive(Clone, Debug, Eq, PartialEq, Serialize, Deserialize)]
#[serde(deny_unknown_fields)]
pub struct WorkspaceCommitPayloadV1 {
    pub workspace_binding_id: String,
    pub workspace_digest: String,
    pub change_set: ExactRefV1,
    pub view: ExactRefV1,
    pub base_revision: String,
    pub file_scope_digest: String,
    pub write_scopes: Vec<String>,
    pub changes: Vec<WorkspaceMutationV1>,
    #[serde(default, skip_serializing_if = "Option::is_none")]
    pub inspection_target: Option<ProviderInspectionTargetV1>,
}

impl WorkspaceCommitPayloadV1 {
    fn validate(&self) -> Result<()> {
        if self.workspace_binding_id.trim().is_empty()
            || !valid_digest(&self.workspace_digest)
            || self.change_set.validate("workspace change set").is_err()
            || self.view.validate("workspace view").is_err()
            || !valid_digest(&self.base_revision)
            || !valid_digest(&self.file_scope_digest)
            || self.write_scopes.is_empty()
            || self.write_scopes.len() > 256
            || self.changes.is_empty()
            || self.changes.len() > 4096
        {
            return Err(ClosedError::new(
                ClosedReason::InvalidArgument,
                "workspace commit payload violates the closed policy",
            ));
        }
        let mut previous_scope = "";
        for scope in &self.write_scopes {
            if !valid_logical_path(scope) || scope.as_str() <= previous_scope {
                return Err(ClosedError::new(
                    ClosedReason::InvalidArgument,
                    "workspace commit scopes are not canonical, sorted, and unique",
                ));
            }
            previous_scope = scope;
        }
        let mut previous_change = String::new();
        let mut touched = BTreeSet::new();
        for change in &self.changes {
            let key = format!("{}\0{}\0{}", change.path, change.kind, change.target_path);
            if !valid_logical_path(&change.path)
                || key <= previous_change
                || !within_scope(&change.path, &self.write_scopes)
                || !touched.insert(change.path.as_str())
            {
                return Err(ClosedError::new(
                    ClosedReason::InvalidArgument,
                    "workspace commit changes are not canonical, unique, and scoped",
                ));
            }
            match change.kind.as_str() {
                "add" | "modify" => {
                    if !change.target_path.is_empty()
                        || !change.blob_id.starts_with("workspace-blob-")
                        || change.blob_id.len() != "workspace-blob-".len() + 64
                        || !valid_digest(&change.blob_digest)
                        || change.mode == 0
                        || change.mode & !0o777 != 0
                    {
                        return Err(ClosedError::new(
                            ClosedReason::InvalidArgument,
                            "workspace add/modify mutation is incomplete",
                        ));
                    }
                }
                "delete" => {
                    if !change.target_path.is_empty()
                        || !change.blob_id.is_empty()
                        || !change.blob_digest.is_empty()
                        || change.mode != 0
                    {
                        return Err(ClosedError::new(
                            ClosedReason::InvalidArgument,
                            "workspace delete mutation carries forbidden fields",
                        ));
                    }
                }
                "rename" => {
                    if !valid_logical_path(&change.target_path)
                        || !within_scope(&change.target_path, &self.write_scopes)
                        || !change.blob_id.is_empty()
                        || !change.blob_digest.is_empty()
                        || change.mode != 0
                        || !touched.insert(change.target_path.as_str())
                    {
                        return Err(ClosedError::new(
                            ClosedReason::InvalidArgument,
                            "workspace rename mutation is incomplete or overlaps another path",
                        ));
                    }
                }
                _ => {
                    return Err(ClosedError::new(
                        ClosedReason::Unsupported,
                        "workspace mutation kind is unsupported",
                    ));
                }
            }
            previous_change = key;
        }
        Ok(())
    }
}

fn valid_logical_path(value: &str) -> bool {
    !value.is_empty()
        && !value.starts_with('/')
        && !value.contains('\\')
        && !value
            .split('/')
            .any(|part| part.is_empty() || part == "." || part == "..")
}

fn within_scope(value: &str, scopes: &[String]) -> bool {
    scopes.iter().any(|scope| {
        value == scope
            || value
                .strip_prefix(scope)
                .is_some_and(|rest| rest.starts_with('/'))
    })
}

#[allow(clippy::trivially_copy_pass_by_ref)] // serde skip_serializing_if requires fn(&T).
const fn is_zero_u32(value: &u32) -> bool {
    *value == 0
}

impl RemotePayloadV1 {
    fn validate(&self) -> Result<()> {
        if self.endpoint_binding_id.trim().is_empty()
            || !valid_digest(&self.endpoint_digest)
            || self.workload_id.trim().is_empty()
            || !valid_digest(&self.workload_digest)
            || self.credential.validate("remote credential").is_err()
            || self.isolation_profile.trim().is_empty()
        {
            return Err(ClosedError::new(
                ClosedReason::InvalidArgument,
                "remote payload violates the closed policy",
            ));
        }
        Ok(())
    }
}

impl MicroVmPayloadV1 {
    fn validate(&self) -> Result<()> {
        if self.kernel_binding_id.trim().is_empty()
            || !valid_digest(&self.kernel_digest)
            || self.initramfs_binding_id.trim().is_empty()
            || !valid_digest(&self.initramfs_digest)
            || self.vcpus == 0
            || self.vcpus > 64
            || self.memory_mib < 64
            || self.memory_mib > 1_048_576
            || !self.network_deny_all
            || self.wall_clock_timeout_millis == 0
            || self.wall_clock_timeout_millis > 604_800_000
        {
            return Err(ClosedError::new(
                ClosedReason::InvalidArgument,
                "microVM payload violates the closed policy",
            ));
        }
        Ok(())
    }
}

#[derive(Clone, Debug, Eq, PartialEq, Serialize, Deserialize)]
#[serde(
    tag = "provider_kind",
    content = "provider_payload",
    rename_all = "snake_case"
)]
pub enum ProviderPayloadV1 {
    HostWorkspace(HostWorkspacePayloadV1),
    QemuMicrovm(MicroVmPayloadV1),
    ContainerdOci(ContainerPayloadV1),
    WasmtimeComponent(WasmPayloadV1),
    RemoteSandbox(RemotePayloadV1),
    WorkspaceCommit(WorkspaceCommitPayloadV1),
}

impl ProviderPayloadV1 {
    fn validate(&self) -> Result<()> {
        match self {
            Self::HostWorkspace(payload) => payload.validate(),
            Self::QemuMicrovm(payload) => payload.validate(),
            Self::ContainerdOci(payload) => payload.validate(),
            Self::WasmtimeComponent(payload) => payload.validate(),
            Self::RemoteSandbox(payload) => payload.validate(),
            Self::WorkspaceCommit(payload) => payload.validate(),
        }
    }

    #[must_use]
    pub const fn kind(&self) -> ProviderKindV1 {
        match self {
            Self::HostWorkspace(_) => ProviderKindV1::HostWorkspace,
            Self::QemuMicrovm(_) => ProviderKindV1::QemuMicrovm,
            Self::ContainerdOci(_) => ProviderKindV1::ContainerdOci,
            Self::WasmtimeComponent(_) => ProviderKindV1::WasmtimeComponent,
            Self::RemoteSandbox(_) => ProviderKindV1::RemoteSandbox,
            Self::WorkspaceCommit(_) => ProviderKindV1::WorkspaceCommit,
        }
    }

    #[must_use]
    pub const fn inspection_target(&self) -> Option<&ProviderInspectionTargetV1> {
        match self {
            Self::HostWorkspace(payload) => payload.inspection_target.as_ref(),
            Self::QemuMicrovm(payload) => payload.inspection_target.as_ref(),
            Self::ContainerdOci(payload) => payload.inspection_target.as_ref(),
            Self::WasmtimeComponent(payload) => payload.inspection_target.as_ref(),
            Self::RemoteSandbox(payload) => payload.inspection_target.as_ref(),
            Self::WorkspaceCommit(payload) => payload.inspection_target.as_ref(),
        }
    }
}

#[derive(Clone, Debug, Eq, PartialEq, Serialize, Deserialize)]
#[serde(deny_unknown_fields)]
pub struct DispatchRequestV1 {
    pub contract_version: String,
    pub request_id: String,
    pub phase: EnforcementPhaseV1,
    pub effect_kind: String,
    pub operation_digest: String,
    pub effect_id: String,
    pub intent_revision: u64,
    pub intent_digest: String,
    pub attempt_id: String,
    pub tenant_id: String,
    pub provider_binding: ProviderBindingV1,
    pub sandbox_attempt: ExactRefV1,
    pub execution_binding: ExecutionBindingV1,
    pub runtime_enforcement: RuntimeEnforcementRefV1,
    pub runtime_current_query: serde_json::Value,
    pub runtime_current_query_digest: String,
    pub requested_not_after_unix_nano: i64,
    pub payload_schema: String,
    pub payload_digest: String,
    pub payload_revision: u64,
    pub payload: ProviderPayloadV1,
    pub digest: String,
}

impl DispatchRequestV1 {
    pub fn validate_shape(&self) -> Result<()> {
        if self.contract_version != CONTRACT_VERSION_V1
            || self.request_id.trim().is_empty()
            || !valid_effect_kind(&self.effect_kind)
            || !valid_digest(&self.operation_digest)
            || self.effect_id.trim().is_empty()
            || self.intent_revision == 0
            || !valid_digest(&self.intent_digest)
            || self.attempt_id.trim().is_empty()
            || self.tenant_id.trim().is_empty()
            || self.requested_not_after_unix_nano <= 0
            || self.payload_schema.trim().is_empty()
            || !valid_digest(&self.payload_digest)
            || self.payload_revision == 0
            || self.payload_digest != canonical_digest("ProviderPayloadV1", &self.payload)?
        {
            return Err(ClosedError::new(
                ClosedReason::InvalidContract,
                "dispatch request identity is incomplete",
            ));
        }
        self.provider_binding.validate()?;
        self.sandbox_attempt.validate("sandbox attempt")?;
        if self.sandbox_attempt.id != self.attempt_id {
            return Err(ClosedError::new(
                ClosedReason::BindingDrift,
                "sandbox attempt ref does not match attempt identity",
            ));
        }
        self.execution_binding.validate()?;
        if self.execution_binding.tenant_id != self.tenant_id {
            return Err(ClosedError::new(
                ClosedReason::BindingDrift,
                "execution binding tenant drifted",
            ));
        }
        self.runtime_enforcement.validate()?;
        if self.runtime_enforcement.operation_digest != self.operation_digest
            || self.runtime_enforcement.effect_id != self.effect_id
            || self.runtime_enforcement.attempt_id != self.attempt_id
            || self.runtime_enforcement.phase != self.phase
        {
            return Err(ClosedError::new(
                ClosedReason::BindingDrift,
                "runtime enforcement ref drifted from dispatch coordinates",
            ));
        }
        if self.runtime_current_query.is_null()
            || self.runtime_current_query_digest
                != canonical_digest("RuntimeCurrentQueryV1", &self.runtime_current_query)?
        {
            return Err(ClosedError::new(
                ClosedReason::InvalidContract,
                "runtime current query is absent or drifted",
            ));
        }
        self.payload.validate()?;
        match (self.effect_kind.as_str(), self.payload.inspection_target()) {
            ("praxis.sandbox/inspect", Some(target)) => {
                target.validate(self.payload.kind(), &self.tenant_id, now_unix_nano())?;
            }
            ("praxis.sandbox/cleanup", Some(target))
                if self.payload.kind() == ProviderKindV1::WorkspaceCommit =>
            {
                target.validate(self.payload.kind(), &self.tenant_id, now_unix_nano())?;
            }
            ("praxis.sandbox/inspect", None) => {
                return Err(ClosedError::new(
                    ClosedReason::InvalidContract,
                    "inspection dispatch lacks an exact original target",
                ));
            }
            (_, Some(_)) => {
                return Err(ClosedError::new(
                    ClosedReason::InvalidContract,
                    "dispatch carries a forbidden inspection target",
                ));
            }
            (_, None) => {}
        }
        if self.digest != self.calculate_digest()? {
            return Err(ClosedError::new(
                ClosedReason::InvalidDigest,
                "dispatch request digest drifted",
            ));
        }
        Ok(())
    }

    pub fn validate_current(&self, now_unix_nano: i64) -> Result<()> {
        self.validate_shape()?;
        self.sandbox_attempt
            .validate_current("sandbox attempt", now_unix_nano)?;
        self.execution_binding.validate_current(now_unix_nano)?;
        self.runtime_enforcement.validate_current(now_unix_nano)?;
        if now_unix_nano <= 0 || now_unix_nano >= self.requested_not_after_unix_nano {
            return Err(ClosedError::new(
                ClosedReason::CurrentExpired,
                "dispatch request TTL is expired",
            ));
        }
        Ok(())
    }

    pub fn calculate_digest(&self) -> Result<String> {
        let mut canonical = self.clone();
        canonical.digest.clear();
        canonical_digest("DispatchRequestV1", &canonical)
    }

    pub fn seal(mut self) -> Result<Self> {
        self.contract_version = CONTRACT_VERSION_V1.to_owned();
        self.payload_digest = canonical_digest("ProviderPayloadV1", &self.payload)?;
        self.runtime_current_query_digest =
            canonical_digest("RuntimeCurrentQueryV1", &self.runtime_current_query)?;
        self.digest.clear();
        self.digest = self.calculate_digest()?;
        self.validate_shape()?;
        Ok(self)
    }
}

#[derive(Clone, Debug, Eq, PartialEq, Serialize, Deserialize)]
#[serde(deny_unknown_fields)]
pub struct CurrentAuthorizationV1 {
    pub contract_version: String,
    pub request_digest: String,
    pub operation_digest: String,
    pub effect_id: String,
    pub attempt_id: String,
    pub phase: EnforcementPhaseV1,
    pub provider_binding: ProviderBindingV1,
    pub sandbox_projection: SandboxProjectionRefV1,
    pub execution_binding: ExecutionBindingV1,
    pub runtime_enforcement: RuntimeEnforcementRefV1,
    pub checked_unix_nano: i64,
    pub expires_unix_nano: i64,
    pub digest: String,
}

#[derive(Clone, Debug, Eq, PartialEq, Serialize, Deserialize)]
#[serde(deny_unknown_fields)]
pub struct CurrentReadResponseV1 {
    pub authorization: Option<CurrentAuthorizationV1>,
    pub error: Option<ClosedError>,
}

impl CurrentAuthorizationV1 {
    pub fn validate_against(&self, request: &DispatchRequestV1, now_unix_nano: i64) -> Result<()> {
        self.provider_binding.validate()?;
        self.runtime_enforcement.validate_current(now_unix_nano)?;
        self.execution_binding.validate_current(now_unix_nano)?;
        self.sandbox_projection.validate_current(now_unix_nano)?;
        if self.contract_version != CONTRACT_VERSION_V1
            || self.request_digest != request.digest
            || self.operation_digest != request.operation_digest
            || self.effect_id != request.effect_id
            || self.attempt_id != request.attempt_id
            || self.phase != request.phase
            || self.provider_binding != request.provider_binding
            || self.execution_binding != request.execution_binding
            || self.runtime_enforcement != request.runtime_enforcement
            || self.checked_unix_nano <= 0
            || self.checked_unix_nano > now_unix_nano
            || now_unix_nano >= self.expires_unix_nano
            || self.expires_unix_nano > request.requested_not_after_unix_nano
            || self.expires_unix_nano > self.sandbox_projection.expires_unix_nano
            || self.expires_unix_nano > self.runtime_enforcement.expires_unix_nano
            || self.expires_unix_nano > self.execution_binding.expires_unix_nano
            || self.digest != self.calculate_digest()?
        {
            return Err(ClosedError::new(
                ClosedReason::BindingDrift,
                "current authorization drifted from exact dispatch request",
            ));
        }
        Ok(())
    }

    pub fn calculate_digest(&self) -> Result<String> {
        let mut canonical = self.clone();
        canonical.digest.clear();
        canonical_digest("CurrentAuthorizationV1", &canonical)
    }
}

#[derive(Clone, Debug, Eq, PartialEq, Serialize, Deserialize)]
#[serde(deny_unknown_fields)]
pub struct DispatchResponseV1 {
    pub contract_version: String,
    pub request_id: String,
    pub request_digest: String,
    pub accepted: bool,
    pub provider_attempt: Option<ExactRefV1>,
    pub provider_observation: Option<crate::provider::ProviderObservation>,
    pub provider_receipt: Option<crate::provider::ProviderReceipt>,
    pub observation_digest: Option<String>,
    pub receipt_digest: Option<String>,
    pub checked_unix_nano: i64,
    pub expires_unix_nano: i64,
    pub error: Option<ClosedError>,
    pub digest: String,
}

impl DispatchResponseV1 {
    pub fn success(
        request: &DispatchRequestV1,
        result: &crate::provider::ProviderResult,
    ) -> Result<Self> {
        result.validate(request, now_unix_nano())?;
        Self {
            contract_version: CONTRACT_VERSION_V1.to_owned(),
            request_id: request.request_id.clone(),
            request_digest: request.digest.clone(),
            accepted: true,
            provider_attempt: Some(result.attempt.clone()),
            provider_observation: Some(result.observation.clone()),
            provider_receipt: Some(result.receipt.clone()),
            observation_digest: Some(result.observation.digest.clone()),
            receipt_digest: Some(result.receipt.digest.clone()),
            checked_unix_nano: now_unix_nano(),
            expires_unix_nano: result.receipt.expires_unix_nano,
            error: None,
            digest: String::new(),
        }
        .seal()
    }

    pub fn failure(request: &DispatchRequestV1, error: ClosedError) -> Result<Self> {
        let now = now_unix_nano();
        Self {
            contract_version: CONTRACT_VERSION_V1.to_owned(),
            request_id: request.request_id.clone(),
            request_digest: request.digest.clone(),
            accepted: false,
            provider_attempt: None,
            provider_observation: None,
            provider_receipt: None,
            observation_digest: None,
            receipt_digest: None,
            checked_unix_nano: now,
            expires_unix_nano: request.requested_not_after_unix_nano.max(now),
            error: Some(error),
            digest: String::new(),
        }
        .seal()
    }

    pub fn calculate_digest(&self) -> Result<String> {
        let mut canonical = self.clone();
        canonical.digest.clear();
        canonical_digest("DispatchResponseV1", &canonical)
    }

    pub fn seal(mut self) -> Result<Self> {
        self.digest.clear();
        self.digest = self.calculate_digest()?;
        Ok(self)
    }
}

pub fn canonical_digest<T: Serialize>(kind: &str, value: &T) -> Result<String> {
    let bytes = serde_json::to_vec(value).map_err(ClosedError::internal)?;
    let mut digest = Sha256::new();
    digest.update(CONTRACT_VERSION_V1.as_bytes());
    digest.update([0]);
    digest.update(kind.as_bytes());
    digest.update([0]);
    digest.update(bytes);
    Ok(format!("sha256:{}", hex::encode(digest.finalize())))
}

#[must_use]
pub fn now_unix_nano() -> i64 {
    let Ok(duration) = SystemTime::now().duration_since(UNIX_EPOCH) else {
        return 0;
    };
    i64::try_from(duration.as_nanos()).unwrap_or(0)
}

#[must_use]
pub fn valid_digest(value: &str) -> bool {
    value.len() == 71
        && value.starts_with("sha256:")
        && value[7..].bytes().all(|byte| byte.is_ascii_hexdigit())
}

#[must_use]
pub fn valid_effect_kind(value: &str) -> bool {
    matches!(
        value,
        "praxis.sandbox/backend-discovery"
            | "praxis.sandbox/allocate"
            | "praxis.sandbox/activate"
            | "praxis.sandbox/open"
            | "praxis.sandbox/cancel"
            | "praxis.sandbox/close"
            | "praxis.sandbox/fence"
            | "praxis.sandbox/release"
            | "praxis.sandbox/inspect"
            | "praxis.sandbox/cleanup"
            | "praxis.sandbox/workspace-commit"
            | "praxis.sandbox/checkpoint"
    )
}

#[must_use]
fn inspectable_original_effect_kind(value: &str) -> bool {
    matches!(
        value,
        "praxis.sandbox/backend-discovery"
            | "praxis.sandbox/allocate"
            | "praxis.sandbox/activate"
            | "praxis.sandbox/open"
            | "praxis.sandbox/cancel"
            | "praxis.sandbox/close"
            | "praxis.sandbox/fence"
            | "praxis.sandbox/release"
            | "praxis.sandbox/cleanup"
            | "praxis.sandbox/workspace-commit"
    )
}

#[cfg(test)]
mod tests {
    use super::inspectable_original_effect_kind;

    #[test]
    fn independent_lifecycle_effects_are_inspectable_without_expanding_special_scopes() {
        for effect in [
            "praxis.sandbox/backend-discovery",
            "praxis.sandbox/allocate",
            "praxis.sandbox/activate",
            "praxis.sandbox/open",
            "praxis.sandbox/cancel",
            "praxis.sandbox/close",
            "praxis.sandbox/fence",
            "praxis.sandbox/release",
            "praxis.sandbox/cleanup",
        ] {
            assert!(inspectable_original_effect_kind(effect), "{effect}");
        }
        for effect in [
            "",
            "praxis.sandbox/inspect",
            "praxis.sandbox/checkpoint",
            "custom/effect",
        ] {
            assert!(!inspectable_original_effect_kind(effect), "{effect}");
        }
    }
}

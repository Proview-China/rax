use std::collections::{BTreeMap, BTreeSet};
use std::path::{Path, PathBuf};
use std::process::Stdio;
use std::sync::atomic::{AtomicU64, Ordering};
use std::time::Duration;

use async_trait::async_trait;
use nix::sys::signal::{Signal, kill};
use nix::unistd::Pid;
use serde::{Deserialize, Serialize};
use sha2::{Digest as _, Sha256};
use tokio::fs;
use tokio::io::AsyncWriteExt as _;
use tokio::process::Command;

use crate::checkpoint::CheckpointSource;
use crate::contract::{
    DispatchRequestV1, HostWorkspacePayloadV1, ProviderKindV1, ProviderPayloadV1,
};
use crate::error::{ClosedError, ClosedReason, Result};
use crate::provider::{Provider, ProviderResult, provider_result};

static STATE_WRITE_SEQUENCE: AtomicU64 = AtomicU64::new(1);

#[derive(Clone, Debug, Deserialize)]
#[serde(deny_unknown_fields)]
pub struct HostWorkspaceBinding {
    pub path: PathBuf,
    pub digest: String,
    pub writable_overlay: bool,
}

#[derive(Clone, Debug, Deserialize)]
#[serde(deny_unknown_fields)]
pub struct HostToolBinding {
    pub path: PathBuf,
    pub digest: String,
}

#[derive(Clone, Debug, Deserialize)]
#[serde(deny_unknown_fields)]
pub struct HostConfig {
    pub bwrap_path: PathBuf,
    pub state_directory: PathBuf,
    pub runtime_readonly_paths: Vec<PathBuf>,
    pub allowed_environment_keys: BTreeSet<String>,
    pub workspace_bindings: BTreeMap<String, HostWorkspaceBinding>,
    pub tool_bindings: BTreeMap<String, HostToolBinding>,
}

#[derive(Clone, Debug, Serialize, Deserialize)]
#[serde(deny_unknown_fields)]
struct AllocationRecord {
    tenant_id: String,
    lease_id: String,
    lease_epoch: u64,
    instance_epoch: u64,
    workspace_binding_id: String,
    workspace_digest: String,
    tool_binding_id: String,
    tool_digest: String,
}

#[derive(Clone, Debug, Serialize, Deserialize)]
#[serde(deny_unknown_fields)]
struct ProcessRecord {
    attempt_id: String,
    payload_digest: String,
    pid: u32,
    start_time_ticks: u64,
    state: String,
}

pub struct HostProvider {
    config: HostConfig,
}

impl HostProvider {
    #[must_use]
    pub fn new(config: HostConfig) -> Self {
        Self { config }
    }

    pub async fn probe(&self) -> Result<()> {
        ensure_regular_file(&self.config.bwrap_path).await?;
        if !self.config.bwrap_path.is_absolute()
            || !self.config.state_directory.is_absolute()
            || self.config.runtime_readonly_paths.is_empty()
            || self
                .config
                .runtime_readonly_paths
                .iter()
                .any(|path| !path.is_absolute())
            || self
                .config
                .allowed_environment_keys
                .iter()
                .any(|key| key.is_empty() || key.contains('=') || key.contains('\0'))
        {
            return Err(ClosedError::new(
                ClosedReason::InvalidContract,
                "host provider configuration is invalid",
            ));
        }
        for path in &self.config.runtime_readonly_paths {
            ensure_real_directory(path).await?;
        }
        fs::create_dir_all(&self.config.state_directory)
            .await
            .map_err(ClosedError::internal)
    }

    async fn validate_binding(
        &self,
        payload: &HostWorkspacePayloadV1,
    ) -> Result<(&HostWorkspaceBinding, &HostToolBinding)> {
        let workspace = self
            .config
            .workspace_bindings
            .get(&payload.workspace_binding_id)
            .ok_or_else(|| {
                ClosedError::new(
                    ClosedReason::NotFoundObservation,
                    "host workspace binding was not found",
                )
            })?;
        let tool = self
            .config
            .tool_bindings
            .get(&payload.tool_binding_id)
            .ok_or_else(|| {
                ClosedError::new(
                    ClosedReason::NotFoundObservation,
                    "host tool binding was not found",
                )
            })?;
        if workspace.digest != payload.workspace_digest || tool.digest != payload.tool_digest {
            return Err(ClosedError::new(
                ClosedReason::InvalidDigest,
                "host workspace or tool binding digest drifted",
            ));
        }
        ensure_real_directory(&workspace.path).await?;
        ensure_regular_file(&tool.path).await?;
        if sha256_file(&tool.path).await? != tool.digest {
            return Err(ClosedError::new(
                ClosedReason::InvalidDigest,
                "host tool content digest drifted",
            ));
        }
        if payload
            .environment
            .keys()
            .any(|key| !self.config.allowed_environment_keys.contains(key))
        {
            return Err(ClosedError::new(
                ClosedReason::InvalidArgument,
                "host payload requested an unapproved environment key",
            ));
        }
        let working_directory = workspace.path.join(&payload.working_directory);
        ensure_real_directory(&working_directory).await?;
        let canonical_workspace = fs::canonicalize(&workspace.path)
            .await
            .map_err(ClosedError::internal)?;
        let canonical_working = fs::canonicalize(working_directory)
            .await
            .map_err(ClosedError::internal)?;
        if !canonical_working.starts_with(&canonical_workspace) {
            return Err(ClosedError::new(
                ClosedReason::BindingDrift,
                "host working directory escaped the workspace binding",
            ));
        }
        Ok((workspace, tool))
    }

    async fn write_allocation(
        &self,
        request: &DispatchRequestV1,
        payload: &HostWorkspacePayloadV1,
    ) -> Result<()> {
        let directory = self.lease_directory(request);
        fs::create_dir_all(&directory)
            .await
            .map_err(ClosedError::internal)?;
        let next = AllocationRecord {
            tenant_id: request.tenant_id.clone(),
            lease_id: request.execution_binding.lease_id.clone(),
            lease_epoch: request.execution_binding.lease_epoch,
            instance_epoch: request.execution_binding.instance_epoch,
            workspace_binding_id: payload.workspace_binding_id.clone(),
            workspace_digest: payload.workspace_digest.clone(),
            tool_binding_id: payload.tool_binding_id.clone(),
            tool_digest: payload.tool_digest.clone(),
        };
        let path = directory.join("allocation.json");
        if let Some(existing) = read_json_optional::<AllocationRecord>(&path).await? {
            if existing == next {
                return Ok(());
            }
            return Err(ClosedError::new(
                ClosedReason::Conflict,
                "host lease is already allocated to different exact bindings",
            ));
        }
        write_json_atomic(&path, &next).await
    }

    async fn require_allocation(
        &self,
        request: &DispatchRequestV1,
        payload: &HostWorkspacePayloadV1,
    ) -> Result<()> {
        let path = self.lease_directory(request).join("allocation.json");
        let existing = read_json_optional::<AllocationRecord>(&path)
            .await?
            .ok_or_else(|| {
                ClosedError::new(
                    ClosedReason::NotFoundObservation,
                    "host workspace lease is not allocated",
                )
            })?;
        if existing.tenant_id != request.tenant_id
            || existing.lease_id != request.execution_binding.lease_id
            || existing.lease_epoch != request.execution_binding.lease_epoch
            || existing.instance_epoch != request.execution_binding.instance_epoch
            || existing.workspace_binding_id != payload.workspace_binding_id
            || existing.workspace_digest != payload.workspace_digest
            || existing.tool_binding_id != payload.tool_binding_id
            || existing.tool_digest != payload.tool_digest
        {
            return Err(ClosedError::new(
                ClosedReason::BindingDrift,
                "host allocation no longer matches the exact lease and payload",
            ));
        }
        Ok(())
    }

    async fn execute_host(&self, request: &DispatchRequestV1) -> Result<ProviderResult> {
        let payload = host_payload(request)?;
        let (workspace, tool) = self.validate_binding(payload).await?;
        self.require_allocation(request, payload).await?;
        let process_path = self.lease_directory(request).join("process.json");
        if let Some(existing) = read_json_optional::<ProcessRecord>(&process_path).await?
            && process_is_current(&existing).await?
        {
            return Err(ClosedError::new(
                ClosedReason::Conflict,
                "host lease already has a live process",
            ));
        }

        let mut command = self.build_command(payload, workspace, tool);
        let mut child = command.spawn().map_err(|_| {
            ClosedError::new(
                ClosedReason::ProviderUnavailable,
                "host workspace executor failed to start",
            )
        })?;
        let pid = child.id().ok_or_else(|| {
            ClosedError::new(
                ClosedReason::ProviderUnknown,
                "host workspace executor omitted process identity",
            )
        })?;
        let start_time_ticks = proc_start_time(pid).await.unwrap_or(0);
        let mut record = ProcessRecord {
            attempt_id: request.attempt_id.clone(),
            payload_digest: request.payload_digest.clone(),
            pid,
            start_time_ticks,
            state: "running".to_owned(),
        };
        write_json_atomic(&process_path, &record).await?;
        let timeout = Duration::from_millis(payload.wall_clock_timeout_millis);
        let state = match tokio::time::timeout(timeout, child.wait()).await {
            Ok(Ok(status)) => status.code().map_or_else(
                || "exited:signal".to_owned(),
                |code| format!("exited:{code}"),
            ),
            Ok(Err(_)) => "exit_unknown".to_owned(),
            Err(_) => {
                let _ = kill_process_group(&record);
                let _ = child.wait().await;
                "timed_out".to_owned()
            }
        };
        record.state.clone_from(&state);
        write_json_atomic(&process_path, &record).await?;
        provider_result(request, &state)
    }

    fn build_command(
        &self,
        payload: &HostWorkspacePayloadV1,
        workspace: &HostWorkspaceBinding,
        tool: &HostToolBinding,
    ) -> Command {
        let mut command = Command::new(&self.config.bwrap_path);
        command
            .arg("--die-with-parent")
            .arg("--unshare-user")
            .arg("--unshare-pid")
            .arg("--unshare-ipc")
            .arg("--unshare-uts")
            .arg("--unshare-net")
            .arg("--new-session")
            .arg("--clearenv")
            .arg("--tmpfs")
            .arg("/")
            .arg("--proc")
            .arg("/proc")
            .arg("--dev")
            .arg("/dev")
            .arg("--dir")
            .arg("/praxis");
        for path in &self.config.runtime_readonly_paths {
            command.arg("--ro-bind").arg(path).arg(path);
        }
        command
            .arg("--symlink")
            .arg("usr/bin")
            .arg("/bin")
            .arg("--symlink")
            .arg("usr/lib")
            .arg("/lib")
            .arg("--symlink")
            .arg("usr/lib64")
            .arg("/lib64");
        command.arg("--ro-bind").arg(&tool.path).arg("/praxis/tool");
        command.arg(if workspace.writable_overlay {
            "--bind"
        } else {
            "--ro-bind"
        });
        command.arg(&workspace.path).arg("/workspace");
        command
            .arg("--chdir")
            .arg(format!("/workspace/{}", payload.working_directory))
            .arg("--setenv")
            .arg("PATH")
            .arg("/usr/bin:/bin");
        for (key, value) in &payload.environment {
            command.arg("--setenv").arg(key).arg(value);
        }
        command.arg("/praxis/tool").args(&payload.argv);
        command
            .stdin(Stdio::null())
            .stdout(Stdio::null())
            .stderr(Stdio::null())
            .kill_on_drop(true)
            .process_group(0);
        command
    }

    async fn inspect_state(&self, request: &DispatchRequestV1) -> Result<String> {
        let payload = host_payload(request)?;
        self.validate_binding(payload).await?;
        let record = read_json_optional::<ProcessRecord>(
            &self.lease_directory(request).join("process.json"),
        )
        .await?;
        let Some(record) = record else {
            return Ok("not_found".to_owned());
        };
        if let Some(target) = payload.inspection_target.as_ref()
            && (record.attempt_id != target.original_attempt_id
                || record.payload_digest != target.original_payload_digest)
        {
            return Err(ClosedError::new(
                ClosedReason::Conflict,
                "host inspection target does not match process provenance",
            ));
        }
        if record.state == "running" {
            if process_is_current(&record).await? {
                return Ok("running".to_owned());
            }
            return Ok("exit_unknown".to_owned());
        }
        if record.state == "fence_signal_sent" && !process_is_current(&record).await? {
            return Ok("fenced".to_owned());
        }
        Ok(record.state)
    }

    async fn fence_current(&self, request: &DispatchRequestV1) -> Result<String> {
        let path = self.lease_directory(request).join("process.json");
        let Some(mut record) = read_json_optional::<ProcessRecord>(&path).await? else {
            return Ok("not_found".to_owned());
        };
        if record.state != "running" {
            return Ok(record.state);
        }
        if !process_is_current(&record).await? {
            record.state = "exit_unknown".to_owned();
            write_json_atomic(&path, &record).await?;
            return Ok(record.state);
        }
        kill_process_group(&record)?;
        record.state = "fence_signal_sent".to_owned();
        write_json_atomic(&path, &record).await?;
        Ok(record.state)
    }

    fn lease_directory(&self, request: &DispatchRequestV1) -> PathBuf {
        let mut digest = Sha256::new();
        digest.update(request.tenant_id.as_bytes());
        digest.update([0]);
        digest.update(request.execution_binding.lease_id.as_bytes());
        digest.update([0]);
        digest.update(request.execution_binding.lease_epoch.to_be_bytes());
        digest.update(request.execution_binding.instance_epoch.to_be_bytes());
        self.config
            .state_directory
            .join(&hex::encode(digest.finalize())[..48])
    }
}

#[async_trait]
impl Provider for HostProvider {
    fn kind(&self) -> ProviderKindV1 {
        ProviderKindV1::HostWorkspace
    }

    async fn prepare(&self, request: &DispatchRequestV1) -> Result<ProviderResult> {
        let payload = host_payload(request)?;
        self.probe().await?;
        self.validate_binding(payload).await?;
        if matches!(
            request.effect_kind.as_str(),
            "praxis.sandbox/activate" | "praxis.sandbox/open"
        ) {
            self.require_allocation(request, payload).await?;
        }
        provider_result(request, "effect_prepared")
    }

    async fn execute_prepared(&self, request: &DispatchRequestV1) -> Result<ProviderResult> {
        match request.effect_kind.as_str() {
            "praxis.sandbox/allocate" => {
                let payload = host_payload(request)?;
                self.validate_binding(payload).await?;
                self.write_allocation(request, payload).await?;
                provider_result(request, "workspace_allocated")
            }
            "praxis.sandbox/activate" | "praxis.sandbox/open" => self.execute_host(request).await,
            _ => Err(ClosedError::new(
                ClosedReason::Unsupported,
                "host effect has no execute-prepared implementation",
            )),
        }
    }

    async fn inspect(&self, request: &DispatchRequestV1) -> Result<ProviderResult> {
        if request
            .payload
            .inspection_target()
            .is_some_and(|target| target.original_effect_kind == "praxis.sandbox/cleanup")
        {
            return self.inspect_cleanup(request).await;
        }
        provider_result(request, &self.inspect_state(request).await?)
    }

    async fn fence(&self, request: &DispatchRequestV1) -> Result<ProviderResult> {
        provider_result(request, &self.fence_current(request).await?)
    }

    async fn release(&self, request: &DispatchRequestV1) -> Result<ProviderResult> {
        let directory = self.lease_directory(request);
        if let Some(record) =
            read_json_optional::<ProcessRecord>(&directory.join("process.json")).await?
            && record.state == "running"
            && process_is_current(&record).await?
        {
            return Err(ClosedError::new(
                ClosedReason::Conflict,
                "host release refuses a live process; cancel or fence is separate",
            ));
        }
        match fs::remove_dir_all(directory).await {
            Ok(()) => provider_result(request, "released"),
            Err(error) if error.kind() == std::io::ErrorKind::NotFound => {
                provider_result(request, "not_found")
            }
            Err(error) => Err(ClosedError::internal(error)),
        }
    }

    async fn cleanup(&self, request: &DispatchRequestV1) -> Result<ProviderResult> {
        let _ = self.fence_current(request).await?;
        let directory = self.lease_directory(request);
        match fs::remove_dir_all(directory).await {
            Ok(()) => self.inspect_cleanup(request).await,
            Err(error) if error.kind() == std::io::ErrorKind::NotFound => {
                self.inspect_cleanup(request).await
            }
            Err(error) => Err(ClosedError::internal(error)),
        }
    }

    async fn inspect_cleanup(&self, request: &DispatchRequestV1) -> Result<ProviderResult> {
        let present = fs::try_exists(self.lease_directory(request))
            .await
            .map_err(ClosedError::internal)?;
        provider_result(
            request,
            if present {
                "residual_present"
            } else {
                "cleanup_absent"
            },
        )
    }

    async fn checkpoint_source(&self, request: &DispatchRequestV1) -> Result<CheckpointSource> {
        let payload = host_payload(request)?;
        let (workspace, _) = self.validate_binding(payload).await?;
        Ok(CheckpointSource::Directory(workspace.path.clone()))
    }
}

fn host_payload(request: &DispatchRequestV1) -> Result<&HostWorkspacePayloadV1> {
    let ProviderPayloadV1::HostWorkspace(payload) = &request.payload else {
        return Err(ClosedError::new(
            ClosedReason::BindingDrift,
            "non-host payload reached host provider",
        ));
    };
    Ok(payload)
}

async fn ensure_real_directory(path: &Path) -> Result<()> {
    let metadata = fs::symlink_metadata(path).await.map_err(|_| {
        ClosedError::new(
            ClosedReason::NotFoundObservation,
            "configured host directory is unavailable",
        )
    })?;
    if !metadata.is_dir() || metadata.file_type().is_symlink() {
        return Err(ClosedError::new(
            ClosedReason::InvalidContract,
            "configured host directory is not a real directory",
        ));
    }
    Ok(())
}

async fn ensure_regular_file(path: &Path) -> Result<()> {
    let metadata = fs::symlink_metadata(path).await.map_err(|_| {
        ClosedError::new(
            ClosedReason::NotFoundObservation,
            "configured host executable is unavailable",
        )
    })?;
    if !metadata.is_file() || metadata.file_type().is_symlink() {
        return Err(ClosedError::new(
            ClosedReason::InvalidContract,
            "configured host executable is not a regular file",
        ));
    }
    Ok(())
}

async fn sha256_file(path: &Path) -> Result<String> {
    let bytes = fs::read(path).await.map_err(ClosedError::internal)?;
    Ok(format!("sha256:{}", hex::encode(Sha256::digest(bytes))))
}

async fn proc_start_time(pid: u32) -> Result<u64> {
    let stat = fs::read_to_string(format!("/proc/{pid}/stat"))
        .await
        .map_err(|_| {
            ClosedError::new(
                ClosedReason::NotFoundObservation,
                "host process identity is unavailable",
            )
        })?;
    let close = stat.rfind(')').ok_or_else(|| {
        ClosedError::new(
            ClosedReason::ProviderUnknown,
            "host process identity is malformed",
        )
    })?;
    stat[close + 1..]
        .split_whitespace()
        .nth(19)
        .ok_or_else(|| {
            ClosedError::new(
                ClosedReason::ProviderUnknown,
                "host process start time is absent",
            )
        })?
        .parse::<u64>()
        .map_err(ClosedError::internal)
}

async fn process_is_current(record: &ProcessRecord) -> Result<bool> {
    if record.start_time_ticks == 0 {
        return Ok(false);
    }
    match proc_start_time(record.pid).await {
        Ok(current) => Ok(current == record.start_time_ticks),
        Err(error) if error.reason == ClosedReason::NotFoundObservation => Ok(false),
        Err(error) => Err(error),
    }
}

fn kill_process_group(record: &ProcessRecord) -> Result<()> {
    let pid = i32::try_from(record.pid).map_err(ClosedError::internal)?;
    kill(Pid::from_raw(-pid), Signal::SIGKILL).map_err(|error| {
        if error == nix::errno::Errno::ESRCH {
            ClosedError::new(
                ClosedReason::NotFoundObservation,
                "host process was absent during fence",
            )
        } else {
            ClosedError::internal(error)
        }
    })
}

async fn read_json_optional<T: for<'de> Deserialize<'de>>(path: &Path) -> Result<Option<T>> {
    match fs::read(path).await {
        Ok(bytes) => serde_json::from_slice(&bytes).map(Some).map_err(|_| {
            ClosedError::new(
                ClosedReason::ProviderUnknown,
                "host provider durable state is malformed",
            )
        }),
        Err(error) if error.kind() == std::io::ErrorKind::NotFound => Ok(None),
        Err(error) => Err(ClosedError::internal(error)),
    }
}

async fn write_json_atomic<T: Serialize>(path: &Path, value: &T) -> Result<()> {
    let bytes = serde_json::to_vec(value).map_err(ClosedError::internal)?;
    let sequence = STATE_WRITE_SEQUENCE.fetch_add(1, Ordering::Relaxed);
    let temporary = path.with_extension(format!("json.next.{}.{}", std::process::id(), sequence));
    let file = fs::OpenOptions::new()
        .create(true)
        .truncate(true)
        .write(true)
        .open(&temporary)
        .await
        .map_err(ClosedError::internal)?;
    let mut file = file;
    file.write_all(&bytes)
        .await
        .map_err(ClosedError::internal)?;
    file.sync_all().await.map_err(ClosedError::internal)?;
    fs::rename(&temporary, path)
        .await
        .map_err(ClosedError::internal)
}

impl PartialEq for AllocationRecord {
    fn eq(&self, other: &Self) -> bool {
        self.tenant_id == other.tenant_id
            && self.lease_id == other.lease_id
            && self.lease_epoch == other.lease_epoch
            && self.instance_epoch == other.instance_epoch
            && self.workspace_binding_id == other.workspace_binding_id
            && self.workspace_digest == other.workspace_digest
            && self.tool_binding_id == other.tool_binding_id
            && self.tool_digest == other.tool_digest
    }
}

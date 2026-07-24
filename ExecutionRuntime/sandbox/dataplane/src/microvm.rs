use std::collections::BTreeMap;
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

use crate::contract::{DispatchRequestV1, MicroVmPayloadV1, ProviderKindV1, ProviderPayloadV1};
use crate::error::{ClosedError, ClosedReason, Result};
use crate::provider::{Provider, ProviderResult, provider_result};

static STATE_WRITE_SEQUENCE: AtomicU64 = AtomicU64::new(1);

#[derive(Clone, Debug, Deserialize)]
#[serde(deny_unknown_fields)]
pub struct MicroVmArtifactBinding {
    pub path: PathBuf,
    pub digest: String,
}

#[derive(Clone, Debug, Deserialize)]
#[serde(deny_unknown_fields)]
pub struct MicroVmConfig {
    pub qemu_path: PathBuf,
    pub qemu_digest: String,
    pub state_directory: PathBuf,
    pub kernel_cmdline: String,
    pub require_kvm: bool,
    pub kernel_bindings: BTreeMap<String, MicroVmArtifactBinding>,
    pub initramfs_bindings: BTreeMap<String, MicroVmArtifactBinding>,
}

#[derive(Clone, Debug, Eq, PartialEq, Serialize, Deserialize)]
#[serde(deny_unknown_fields)]
struct AllocationRecord {
    tenant_id: String,
    lease_id: String,
    lease_epoch: u64,
    instance_epoch: u64,
    kernel_binding_id: String,
    kernel_digest: String,
    initramfs_binding_id: String,
    initramfs_digest: String,
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

pub struct MicroVmProvider {
    config: MicroVmConfig,
}

impl MicroVmProvider {
    #[must_use]
    pub fn new(config: MicroVmConfig) -> Self {
        Self { config }
    }

    pub async fn probe(&self) -> Result<()> {
        ensure_regular_file(&self.config.qemu_path).await?;
        if !self.config.qemu_path.is_absolute()
            || !self.config.state_directory.is_absolute()
            || !self.config.require_kvm
            || self.config.kernel_cmdline.trim().is_empty()
            || self.config.kernel_cmdline.contains('\0')
            || sha256_file(&self.config.qemu_path).await? != self.config.qemu_digest
        {
            return Err(ClosedError::new(
                ClosedReason::InvalidContract,
                "microVM provider configuration is invalid or unpinned",
            ));
        }
        let kvm = fs::OpenOptions::new()
            .read(true)
            .write(true)
            .open("/dev/kvm")
            .await
            .map_err(|_| {
                ClosedError::new(
                    ClosedReason::ProviderUnavailable,
                    "required KVM device is unavailable",
                )
            })?;
        drop(kvm);
        fs::create_dir_all(&self.config.state_directory)
            .await
            .map_err(ClosedError::internal)
    }

    async fn validate_binding(
        &self,
        payload: &MicroVmPayloadV1,
    ) -> Result<(&MicroVmArtifactBinding, &MicroVmArtifactBinding)> {
        let kernel = self
            .config
            .kernel_bindings
            .get(&payload.kernel_binding_id)
            .ok_or_else(|| {
                ClosedError::new(
                    ClosedReason::NotFoundObservation,
                    "microVM kernel binding was not found",
                )
            })?;
        let initramfs = self
            .config
            .initramfs_bindings
            .get(&payload.initramfs_binding_id)
            .ok_or_else(|| {
                ClosedError::new(
                    ClosedReason::NotFoundObservation,
                    "microVM initramfs binding was not found",
                )
            })?;
        if kernel.digest != payload.kernel_digest || initramfs.digest != payload.initramfs_digest {
            return Err(ClosedError::new(
                ClosedReason::InvalidDigest,
                "microVM artifact binding digest drifted",
            ));
        }
        for binding in [kernel, initramfs] {
            ensure_regular_file(&binding.path).await?;
            if sha256_file(&binding.path).await? != binding.digest {
                return Err(ClosedError::new(
                    ClosedReason::InvalidDigest,
                    "microVM artifact content digest drifted",
                ));
            }
        }
        Ok((kernel, initramfs))
    }

    async fn write_allocation(
        &self,
        request: &DispatchRequestV1,
        payload: &MicroVmPayloadV1,
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
            kernel_binding_id: payload.kernel_binding_id.clone(),
            kernel_digest: payload.kernel_digest.clone(),
            initramfs_binding_id: payload.initramfs_binding_id.clone(),
            initramfs_digest: payload.initramfs_digest.clone(),
        };
        let path = directory.join("allocation.json");
        if let Some(existing) = read_json_optional::<AllocationRecord>(&path).await? {
            if existing == next {
                return Ok(());
            }
            return Err(ClosedError::new(
                ClosedReason::Conflict,
                "microVM lease is already allocated to different artifacts",
            ));
        }
        if write_json_create_once(&path, &next).await? {
            return Ok(());
        }
        match read_json_optional::<AllocationRecord>(&path).await? {
            Some(existing) if existing == next => Ok(()),
            Some(_) => Err(ClosedError::new(
                ClosedReason::Conflict,
                "microVM lease concurrently selected different artifacts",
            )),
            None => Err(ClosedError::new(
                ClosedReason::ProviderUnknown,
                "microVM allocation winner is not inspectable",
            )),
        }
    }

    async fn require_allocation(
        &self,
        request: &DispatchRequestV1,
        payload: &MicroVmPayloadV1,
    ) -> Result<()> {
        let path = self.lease_directory(request).join("allocation.json");
        let existing = read_json_optional::<AllocationRecord>(&path)
            .await?
            .ok_or_else(|| {
                ClosedError::new(
                    ClosedReason::NotFoundObservation,
                    "microVM lease is not allocated",
                )
            })?;
        let expected = AllocationRecord {
            tenant_id: request.tenant_id.clone(),
            lease_id: request.execution_binding.lease_id.clone(),
            lease_epoch: request.execution_binding.lease_epoch,
            instance_epoch: request.execution_binding.instance_epoch,
            kernel_binding_id: payload.kernel_binding_id.clone(),
            kernel_digest: payload.kernel_digest.clone(),
            initramfs_binding_id: payload.initramfs_binding_id.clone(),
            initramfs_digest: payload.initramfs_digest.clone(),
        };
        if existing != expected {
            return Err(ClosedError::new(
                ClosedReason::BindingDrift,
                "microVM allocation no longer matches the exact lease and payload",
            ));
        }
        Ok(())
    }

    async fn boot(&self, request: &DispatchRequestV1) -> Result<ProviderResult> {
        let payload = microvm_payload(request)?;
        let (kernel, initramfs) = self.validate_binding(payload).await?;
        self.require_allocation(request, payload).await?;
        let directory = self.lease_directory(request);
        let process_path = directory.join("process.json");
        if let Some(existing) = read_json_optional::<ProcessRecord>(&process_path).await?
            && process_is_current(&existing).await?
        {
            return Err(ClosedError::new(
                ClosedReason::Conflict,
                "microVM lease already has a live VM",
            ));
        }
        let activation_lock = directory.join("activation.lock");
        if !write_bytes_create_once(&activation_lock, request.attempt_id.as_bytes()).await? {
            return Err(ClosedError::new(
                ClosedReason::Conflict,
                "microVM activation already began for this lease; inspect the original attempt",
            ));
        }
        let serial_path = directory.join("serial.log");
        let mut command = self.qemu_command(payload, kernel, initramfs, &serial_path);
        let Ok(mut child) = command.spawn() else {
            let _ = fs::remove_file(&activation_lock).await;
            return Err(ClosedError::new(
                ClosedReason::ProviderUnavailable,
                "microVM process failed to start",
            ));
        };
        let Some(pid) = child.id() else {
            let _ = child.kill().await;
            let _ = child.wait().await;
            return Err(ClosedError::new(
                ClosedReason::ProviderUnknown,
                "microVM process omitted identity",
            ));
        };
        let start_time_ticks = match proc_start_time(pid).await {
            Ok(value) => value,
            Err(error) => {
                let _ = child.kill().await;
                let _ = child.wait().await;
                return Err(error);
            }
        };
        let mut record = ProcessRecord {
            attempt_id: request.attempt_id.clone(),
            payload_digest: request.payload_digest.clone(),
            pid,
            start_time_ticks,
            state: "running".to_owned(),
        };
        if let Err(error) = write_json_atomic(&process_path, &record).await {
            let _ = kill_process_group(&record);
            let _ = child.wait().await;
            return Err(error);
        }
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

    fn qemu_command(
        &self,
        payload: &MicroVmPayloadV1,
        kernel: &MicroVmArtifactBinding,
        initramfs: &MicroVmArtifactBinding,
        serial_path: &Path,
    ) -> Command {
        let mut command = Command::new(&self.config.qemu_path);
        command
            .arg("-machine")
            .arg("microvm,accel=kvm")
            .arg("-cpu")
            .arg("host")
            .arg("-nodefaults")
            .arg("-no-user-config")
            .arg("-nographic")
            .arg("-display")
            .arg("none")
            .arg("-monitor")
            .arg("none")
            .arg("-serial")
            .arg(format!("file:{}", serial_path.display()))
            .arg("-net")
            .arg("none")
            .arg("-no-reboot")
            .arg("-sandbox")
            .arg("on,obsolete=deny,elevateprivileges=deny,spawn=deny,resourcecontrol=deny")
            .arg("-kernel")
            .arg(&kernel.path)
            .arg("-initrd")
            .arg(&initramfs.path)
            .arg("-append")
            .arg(&self.config.kernel_cmdline)
            .arg("-m")
            .arg(payload.memory_mib.to_string())
            .arg("-smp")
            .arg(payload.vcpus.to_string())
            .stdin(Stdio::null())
            .stdout(Stdio::null())
            .stderr(Stdio::null())
            .kill_on_drop(true)
            .process_group(0);
        command
    }

    async fn inspect_state(&self, request: &DispatchRequestV1) -> Result<String> {
        let payload = microvm_payload(request)?;
        self.validate_binding(payload).await?;
        let Some(record) = read_json_optional::<ProcessRecord>(
            &self.lease_directory(request).join("process.json"),
        )
        .await?
        else {
            return Ok("not_found".to_owned());
        };
        if let Some(target) = payload.inspection_target.as_ref()
            && (record.attempt_id != target.original_attempt_id
                || record.payload_digest != target.original_payload_digest)
        {
            return Err(ClosedError::new(
                ClosedReason::Conflict,
                "microVM inspection target does not match process provenance",
            ));
        }
        if record.state == "running" {
            return if process_is_current(&record).await? {
                Ok("running".to_owned())
            } else {
                Ok("exit_unknown".to_owned())
            };
        }
        Ok(record.state)
    }

    async fn fence_current(&self, request: &DispatchRequestV1) -> Result<String> {
        let path = self.lease_directory(request).join("process.json");
        let Some(mut record) = read_json_optional::<ProcessRecord>(&path).await? else {
            return Ok("not_found".to_owned());
        };
        if !process_is_current(&record).await? {
            record.state = "fenced".to_owned();
            write_json_atomic(&path, &record).await?;
            return Ok(record.state);
        }
        if record.state == "running" {
            kill_process_group(&record)?;
            record.state = "fence_signal_sent".to_owned();
            write_json_atomic(&path, &record).await?;
        }
        for _ in 0..100 {
            if !process_is_current(&record).await? {
                record.state = "fenced".to_owned();
                write_json_atomic(&path, &record).await?;
                return Ok(record.state);
            }
            tokio::time::sleep(Duration::from_millis(10)).await;
        }
        record.state = "fence_pending".to_owned();
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
impl Provider for MicroVmProvider {
    fn kind(&self) -> ProviderKindV1 {
        ProviderKindV1::QemuMicrovm
    }

    async fn prepare(&self, request: &DispatchRequestV1) -> Result<ProviderResult> {
        let payload = microvm_payload(request)?;
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
                let payload = microvm_payload(request)?;
                self.validate_binding(payload).await?;
                self.write_allocation(request, payload).await?;
                provider_result(request, "microvm_allocated")
            }
            "praxis.sandbox/activate" | "praxis.sandbox/open" => self.boot(request).await,
            _ => Err(ClosedError::new(
                ClosedReason::Unsupported,
                "microVM effect has no execute-prepared implementation",
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
            && process_is_current(&record).await?
        {
            return Err(ClosedError::new(
                ClosedReason::Conflict,
                "microVM release refuses a live VM; fence is separate",
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
        if let Some(record) =
            read_json_optional::<ProcessRecord>(&directory.join("process.json")).await?
            && process_is_current(&record).await?
        {
            return provider_result(request, "residual_present");
        }
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
}

fn microvm_payload(request: &DispatchRequestV1) -> Result<&MicroVmPayloadV1> {
    let ProviderPayloadV1::QemuMicrovm(payload) = &request.payload else {
        return Err(ClosedError::new(
            ClosedReason::BindingDrift,
            "non-microVM payload reached microVM provider",
        ));
    };
    Ok(payload)
}

async fn ensure_regular_file(path: &Path) -> Result<()> {
    let metadata = fs::symlink_metadata(path).await.map_err(|_| {
        ClosedError::new(
            ClosedReason::NotFoundObservation,
            "configured microVM artifact is unavailable",
        )
    })?;
    if !metadata.is_file() || metadata.file_type().is_symlink() {
        return Err(ClosedError::new(
            ClosedReason::InvalidContract,
            "configured microVM artifact is not a regular file",
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
                "microVM process identity is unavailable",
            )
        })?;
    let close = stat.rfind(')').ok_or_else(|| {
        ClosedError::new(
            ClosedReason::ProviderUnknown,
            "microVM process identity is malformed",
        )
    })?;
    stat[close + 1..]
        .split_whitespace()
        .nth(19)
        .ok_or_else(|| {
            ClosedError::new(
                ClosedReason::ProviderUnknown,
                "microVM process start time is absent",
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
                "microVM process was absent during fence",
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
                "microVM provider durable state is malformed",
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
    let mut file = fs::OpenOptions::new()
        .create(true)
        .truncate(true)
        .write(true)
        .open(&temporary)
        .await
        .map_err(ClosedError::internal)?;
    file.write_all(&bytes)
        .await
        .map_err(ClosedError::internal)?;
    file.sync_all().await.map_err(ClosedError::internal)?;
    fs::rename(&temporary, path)
        .await
        .map_err(ClosedError::internal)
}

async fn write_json_create_once<T: Serialize>(path: &Path, value: &T) -> Result<bool> {
    let bytes = serde_json::to_vec(value).map_err(ClosedError::internal)?;
    write_bytes_create_once(path, &bytes).await
}

async fn write_bytes_create_once(path: &Path, bytes: &[u8]) -> Result<bool> {
    let sequence = STATE_WRITE_SEQUENCE.fetch_add(1, Ordering::Relaxed);
    let temporary = path.with_extension(format!("new.{}.{}", std::process::id(), sequence));
    let mut file = fs::OpenOptions::new()
        .create_new(true)
        .write(true)
        .open(&temporary)
        .await
        .map_err(ClosedError::internal)?;
    file.write_all(bytes).await.map_err(ClosedError::internal)?;
    file.sync_all().await.map_err(ClosedError::internal)?;
    drop(file);
    let linked = match fs::hard_link(&temporary, path).await {
        Ok(()) => true,
        Err(error) if error.kind() == std::io::ErrorKind::AlreadyExists => false,
        Err(error) => {
            let _ = fs::remove_file(&temporary).await;
            return Err(ClosedError::internal(error));
        }
    };
    fs::remove_file(&temporary)
        .await
        .map_err(ClosedError::internal)?;
    Ok(linked)
}

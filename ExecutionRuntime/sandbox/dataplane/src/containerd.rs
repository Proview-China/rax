use std::collections::{BTreeMap, HashMap};
use std::path::{Path, PathBuf};

use async_trait::async_trait;
use containerd_client::Client;
use containerd_client::services::v1::{
    Container, CreateContainerRequest, CreateTaskRequest, DeleteContainerRequest,
    DeleteTaskRequest, GetContainerRequest, GetRequest, KillRequest, StartRequest,
    container::Runtime,
};
use prost_types::Any;
use serde::{Deserialize, Serialize};
use serde_json::json;
use sha2::{Digest as _, Sha256};
use tokio::fs;
use tonic::metadata::MetadataValue;
use tonic::{Code, Request, Status};

use crate::checkpoint::CheckpointSource;
use crate::contract::{ContainerPayloadV1, DispatchRequestV1, ProviderKindV1, ProviderPayloadV1};
use crate::error::{ClosedError, ClosedReason, Result};
use crate::provider::{Provider, ProviderResult, provider_result};

const LABEL_ATTEMPT: &str = "praxis.sandbox/allocation-attempt";
const LABEL_PAYLOAD: &str = "praxis.sandbox/allocation-payload-digest";
const LABEL_TENANT: &str = "praxis.sandbox/tenant";
const LABEL_LEASE: &str = "praxis.sandbox/lease";
const LABEL_LEASE_EPOCH: &str = "praxis.sandbox/lease-epoch";
const LABEL_SCOPE: &str = "praxis.sandbox/scope-digest";

#[derive(Clone, Debug, Eq, PartialEq, Serialize, Deserialize)]
#[serde(deny_unknown_fields)]
pub struct RootfsBinding {
    pub path: PathBuf,
    pub image_digest: String,
}

#[derive(Clone, Debug, Serialize, Deserialize)]
#[serde(deny_unknown_fields)]
pub struct ContainerdConfig {
    pub socket_path: PathBuf,
    pub namespace: String,
    pub runtime_name: String,
    pub state_directory: PathBuf,
    pub rootfs_bindings: BTreeMap<String, RootfsBinding>,
}

impl Default for ContainerdConfig {
    fn default() -> Self {
        Self {
            socket_path: PathBuf::from("/run/containerd/containerd.sock"),
            namespace: "praxis".to_owned(),
            runtime_name: "io.containerd.runc.v2".to_owned(),
            state_directory: PathBuf::from("/var/lib/praxis/sandbox/containerd"),
            rootfs_bindings: BTreeMap::new(),
        }
    }
}

pub struct ContainerdProvider {
    config: ContainerdConfig,
}

impl ContainerdProvider {
    #[must_use]
    pub fn new(config: ContainerdConfig) -> Self {
        Self { config }
    }

    pub async fn probe(&self) -> Result<String> {
        self.validate_config()?;
        let client = self.client().await?;
        let response = client.version().version(()).await.map_err(|_| {
            ClosedError::new(
                ClosedReason::ProviderUnavailable,
                "containerd version probe failed",
            )
        })?;
        let version = response.into_inner().version;
        if version.trim().is_empty() {
            return Err(ClosedError::new(
                ClosedReason::ProviderUnknown,
                "containerd returned an empty version",
            ));
        }
        Ok(version)
    }

    fn validate_config(&self) -> Result<()> {
        if self.config.namespace.trim().is_empty()
            || self.config.runtime_name != "io.containerd.runc.v2"
            || self.config.state_directory.as_os_str().is_empty()
        {
            return Err(ClosedError::new(
                ClosedReason::InvalidArgument,
                "containerd provider configuration is invalid",
            ));
        }
        Ok(())
    }

    async fn client(&self) -> Result<Client> {
        self.validate_config()?;
        Client::from_path(&self.config.socket_path)
            .await
            .map_err(|_| {
                ClosedError::new(
                    ClosedReason::ProviderUnavailable,
                    "containerd socket is unavailable",
                )
            })
    }

    async fn bound_rootfs(&self, payload: &ContainerPayloadV1) -> Result<PathBuf> {
        let Some(binding) = self.config.rootfs_bindings.get(&payload.rootfs_binding_id) else {
            return Err(ClosedError::new(
                ClosedReason::NotFoundObservation,
                "rootfs binding was not found",
            ));
        };
        if binding.image_digest != payload.image_digest {
            return Err(ClosedError::new(
                ClosedReason::InvalidDigest,
                "rootfs image digest drifted",
            ));
        }
        let metadata = fs::symlink_metadata(&binding.path).await.map_err(|_| {
            ClosedError::new(
                ClosedReason::NotFoundObservation,
                "bound rootfs is unavailable",
            )
        })?;
        if !metadata.is_dir() || metadata.file_type().is_symlink() {
            return Err(ClosedError::new(
                ClosedReason::InvalidArgument,
                "bound rootfs is not a real directory",
            ));
        }
        fs::canonicalize(&binding.path)
            .await
            .map_err(ClosedError::internal)
    }

    async fn inspect_state(&self, request: &DispatchRequestV1) -> Result<String> {
        let client = self.client().await?;
        let id = container_id(request);
        let get_container = namespaced(
            GetContainerRequest { id: id.clone() },
            &self.config.namespace,
        )?;
        let container = match client.containers().get(get_container).await {
            Ok(response) => response.into_inner().container.ok_or_else(|| {
                ClosedError::new(
                    ClosedReason::ProviderUnknown,
                    "containerd response omitted container",
                )
            })?,
            Err(status) if status.code() == Code::NotFound => return Ok("not_found".to_owned()),
            Err(status) => return Err(map_status(status)),
        };
        validate_container_inspection_target(&container, request)?;
        let get_task = namespaced(
            GetRequest {
                container_id: id.clone(),
                exec_id: String::new(),
            },
            &self.config.namespace,
        )?;
        match client.tasks().get(get_task).await {
            Ok(response) => {
                let process = response.into_inner().process.ok_or_else(|| {
                    ClosedError::new(
                        ClosedReason::ProviderUnknown,
                        "containerd task response omitted process state",
                    )
                })?;
                let state = containerd_client::types::v1::Status::try_from(process.status)
                    .map_or("unknown", |status| status.as_str_name());
                Ok(format!(
                    "task:{}:pid:{}",
                    state.to_ascii_lowercase(),
                    process.pid
                ))
            }
            Err(status) if status.code() == Code::NotFound => Ok("container_prepared".to_owned()),
            Err(status) => Err(map_status(status)),
        }
    }

    async fn create_io_files(
        &self,
        request: &DispatchRequestV1,
    ) -> Result<(String, String, String)> {
        let directory = self.config.state_directory.join(container_id(request));
        fs::create_dir_all(&directory)
            .await
            .map_err(ClosedError::internal)?;
        let directory_metadata = fs::symlink_metadata(&directory)
            .await
            .map_err(ClosedError::internal)?;
        if !directory_metadata.is_dir() || directory_metadata.file_type().is_symlink() {
            return Err(ClosedError::new(
                ClosedReason::Conflict,
                "container IO directory is not an owned real directory",
            ));
        }
        let mut paths = Vec::with_capacity(3);
        for name in ["stdin", "stdout", "stderr"] {
            let path = directory.join(name);
            let _file = fs::OpenOptions::new()
                .create(true)
                .append(true)
                .mode(0o600)
                .custom_flags(nix::libc::O_NOFOLLOW | nix::libc::O_CLOEXEC)
                .open(&path)
                .await
                .map_err(ClosedError::internal)?;
            paths.push(path_to_string(&path)?);
        }
        Ok((paths[0].clone(), paths[1].clone(), paths[2].clone()))
    }

    async fn allocate(&self, request: &DispatchRequestV1) -> Result<ProviderResult> {
        let payload = container_payload(request)?;
        let rootfs = self.bound_rootfs(payload).await?;
        let client = self.client().await?;
        let container = build_container(&self.config, request, payload, &rootfs)?;
        let create = namespaced(
            CreateContainerRequest {
                container: Some(container),
            },
            &self.config.namespace,
        )?;
        match client.containers().create(create).await {
            Ok(_) => provider_result(request, "container_allocated"),
            Err(status) if status.code() == Code::AlreadyExists => {
                let get = namespaced(
                    GetContainerRequest {
                        id: container_id(request),
                    },
                    &self.config.namespace,
                )?;
                let existing = client
                    .containers()
                    .get(get)
                    .await
                    .map_err(map_status)?
                    .into_inner()
                    .container
                    .ok_or_else(|| {
                        ClosedError::new(
                            ClosedReason::ProviderUnknown,
                            "existing container has no inspectable identity",
                        )
                    })?;
                validate_container_identity(&existing, request)?;
                provider_result(request, "container_allocated")
            }
            Err(status) => Err(map_status(status)),
        }
    }

    async fn start(&self, request: &DispatchRequestV1) -> Result<ProviderResult> {
        let client = self.client().await?;
        let (stdin, stdout, stderr) = self.create_io_files(request).await?;
        let id = container_id(request);
        let create = namespaced(
            CreateTaskRequest {
                container_id: id.clone(),
                stdin,
                stdout,
                stderr,
                ..Default::default()
            },
            &self.config.namespace,
        )?;
        client.tasks().create(create).await.map_err(map_status)?;
        let start = namespaced(
            StartRequest {
                container_id: id,
                exec_id: String::new(),
            },
            &self.config.namespace,
        )?;
        let response = client.tasks().start(start).await.map_err(map_status)?;
        provider_result(
            request,
            &format!("task_running:pid:{}", response.into_inner().pid),
        )
    }
}

#[async_trait]
impl Provider for ContainerdProvider {
    fn kind(&self) -> ProviderKindV1 {
        ProviderKindV1::ContainerdOci
    }

    async fn prepare(&self, request: &DispatchRequestV1) -> Result<ProviderResult> {
        let payload = container_payload(request)?;
        match request.effect_kind.as_str() {
            "praxis.sandbox/allocate" => {
                let rootfs = self.bound_rootfs(payload).await?;
                let _container = build_container(&self.config, request, payload, &rootfs)?;
                provider_result(request, "allocation_prepared")
            }
            "praxis.sandbox/activate" | "praxis.sandbox/open" => {
                let state = self.inspect_state(request).await?;
                if state != "container_prepared" && state != "container_allocated" {
                    return Err(ClosedError::new(
                        ClosedReason::Conflict,
                        "container activation requires an allocated exact attempt",
                    ));
                }
                provider_result(request, "activation_prepared")
            }
            _ => {
                self.client().await?;
                provider_result(request, "effect_prepared")
            }
        }
    }

    async fn execute_prepared(&self, request: &DispatchRequestV1) -> Result<ProviderResult> {
        match request.effect_kind.as_str() {
            "praxis.sandbox/allocate" => self.allocate(request).await,
            "praxis.sandbox/activate" | "praxis.sandbox/open" => self.start(request).await,
            _ => Err(ClosedError::new(
                ClosedReason::Unsupported,
                "container effect has no execute-prepared implementation",
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
        let client = self.client().await?;
        let kill = namespaced(
            KillRequest {
                container_id: container_id(request),
                exec_id: String::new(),
                signal: 9,
                all: true,
            },
            &self.config.namespace,
        )?;
        match client.tasks().kill(kill).await {
            Ok(_) => provider_result(request, "fence_signal_sent"),
            Err(status) if status.code() == Code::NotFound => provider_result(request, "not_found"),
            Err(status) => Err(map_status(status)),
        }
    }

    async fn release(&self, request: &DispatchRequestV1) -> Result<ProviderResult> {
        let client = self.client().await?;
        let id = container_id(request);
        let delete_task = namespaced(
            DeleteTaskRequest {
                container_id: id.clone(),
            },
            &self.config.namespace,
        )?;
        match client.tasks().delete(delete_task).await {
            Ok(_) => {}
            Err(status) if status.code() == Code::NotFound => {}
            Err(status) => return Err(map_status(status)),
        }
        let delete_container = namespaced(
            DeleteContainerRequest { id: id.clone() },
            &self.config.namespace,
        )?;
        match client.containers().delete(delete_container).await {
            Ok(_) => {}
            Err(status) if status.code() == Code::NotFound => {}
            Err(status) => return Err(map_status(status)),
        }
        let state_path = self.config.state_directory.join(id);
        match fs::remove_dir_all(state_path).await {
            Ok(()) => {}
            Err(error) if error.kind() == std::io::ErrorKind::NotFound => {}
            Err(error) => return Err(ClosedError::internal(error)),
        }
        provider_result(request, "released")
    }

    async fn cleanup(&self, request: &DispatchRequestV1) -> Result<ProviderResult> {
        let state = self.inspect_state(request).await?;
        if state.starts_with("task:running:")
            || state.starts_with("task:paused:")
            || state.starts_with("task:pausing:")
        {
            self.fence(request).await?;
        }
        self.release(request).await?;
        self.inspect_cleanup(request).await
    }

    async fn inspect_cleanup(&self, request: &DispatchRequestV1) -> Result<ProviderResult> {
        let state = self.inspect_state(request).await?;
        let io_present = fs::try_exists(self.config.state_directory.join(container_id(request)))
            .await
            .map_err(ClosedError::internal)?;
        let cleaned = state == "not_found" && !io_present;
        provider_result(
            request,
            if cleaned {
                "cleanup_absent"
            } else {
                "residual_present"
            },
        )
    }

    async fn checkpoint_source(&self, request: &DispatchRequestV1) -> Result<CheckpointSource> {
        let payload = container_payload(request)?;
        Ok(CheckpointSource::Directory(
            self.bound_rootfs(payload).await?,
        ))
    }
}

fn build_container(
    config: &ContainerdConfig,
    request: &DispatchRequestV1,
    payload: &ContainerPayloadV1,
    rootfs: &Path,
) -> Result<Container> {
    let environment: Vec<String> = payload
        .environment
        .iter()
        .map(|(key, value)| format!("{key}={value}"))
        .collect();
    let rootfs = path_to_string(rootfs)?;
    let spec = json!({
        "ociVersion": "1.1.0",
        "process": {
            "terminal": false,
            "user": {"uid": 65534, "gid": 65534},
            "args": payload.argv,
            "env": environment,
            "cwd": payload.working_directory,
            "capabilities": {
                "bounding": [], "effective": [], "inheritable": [],
                "permitted": [], "ambient": []
            },
            "noNewPrivileges": true,
            "rlimits": [{"type": "RLIMIT_NOFILE", "hard": 1024, "soft": 1024}]
        },
        "root": {"path": rootfs, "readonly": payload.read_only_rootfs},
        "hostname": "praxis-sandbox",
        "mounts": [
            {"destination": "/proc", "type": "proc", "source": "proc", "options": ["nosuid", "noexec", "nodev"]},
            {"destination": "/dev", "type": "tmpfs", "source": "tmpfs", "options": ["nosuid", "strictatime", "mode=755", "size=65536k"]}
        ],
        "linux": {
            "namespaces": [
                {"type": "pid"}, {"type": "ipc"}, {"type": "uts"},
                {"type": "mount"}, {"type": "network"}, {"type": "cgroup"}
            ],
            "resources": {
                "memory": {"limit": payload.memory_limit_bytes},
                "cpu": {"quota": payload.cpu_quota_micros, "period": 100_000},
                "pids": {"limit": payload.pids_limit}
            },
            "seccomp": {
                "defaultAction": "SCMP_ACT_ALLOW",
                "architectures": ["SCMP_ARCH_X86_64", "SCMP_ARCH_X86", "SCMP_ARCH_X32"],
                "syscalls": [{
                    "names": [
                        "acct", "add_key", "bpf", "delete_module", "finit_module", "init_module",
                        "ioperm", "iopl", "kcmp", "kexec_file_load", "kexec_load", "keyctl",
                        "lookup_dcookie", "mount", "move_mount", "name_to_handle_at", "nfsservctl",
                        "open_by_handle_at", "open_tree", "perf_event_open", "pivot_root", "process_vm_readv",
                        "process_vm_writev", "ptrace", "quotactl", "reboot", "request_key", "setns",
                        "sgetmask", "ssetmask", "swapoff", "swapon", "syslog", "umount", "umount2",
                        "unshare", "userfaultfd", "uselib", "vm86", "vm86old"
                    ],
                    "action": "SCMP_ACT_ERRNO"
                }]
            },
            "maskedPaths": ["/proc/acpi", "/proc/kcore", "/proc/keys", "/proc/latency_stats", "/proc/timer_list", "/proc/timer_stats", "/proc/sched_debug", "/sys/firmware"],
            "readonlyPaths": ["/proc/asound", "/proc/bus", "/proc/fs", "/proc/irq", "/proc/sys", "/proc/sysrq-trigger"]
        }
    });
    let labels = HashMap::from([
        (LABEL_ATTEMPT.to_owned(), request.attempt_id.clone()),
        (LABEL_PAYLOAD.to_owned(), request.payload_digest.clone()),
        (LABEL_TENANT.to_owned(), request.tenant_id.clone()),
        (
            LABEL_LEASE.to_owned(),
            request.execution_binding.lease_id.clone(),
        ),
        (
            LABEL_LEASE_EPOCH.to_owned(),
            request.execution_binding.lease_epoch.to_string(),
        ),
        (
            LABEL_SCOPE.to_owned(),
            request.execution_binding.scope_digest.clone(),
        ),
    ]);
    Ok(Container {
        id: container_id(request),
        labels,
        runtime: Some(Runtime {
            name: config.runtime_name.clone(),
            options: None,
        }),
        spec: Some(Any {
            type_url: "types.containerd.io/opencontainers/runtime-spec/1/Spec".to_owned(),
            value: serde_json::to_vec(&spec).map_err(ClosedError::internal)?,
        }),
        ..Default::default()
    })
}

fn validate_container_identity(container: &Container, request: &DispatchRequestV1) -> Result<()> {
    validate_container_ownership(container, request)?;
    if container.labels.get(LABEL_ATTEMPT) != Some(&request.attempt_id)
        || container.labels.get(LABEL_PAYLOAD) != Some(&request.payload_digest)
    {
        return Err(ClosedError::new(
            ClosedReason::Conflict,
            "existing container identity does not match the exact attempt",
        ));
    }
    Ok(())
}

fn validate_container_ownership(container: &Container, request: &DispatchRequestV1) -> Result<()> {
    if container.id != container_id(request)
        || container.labels.get(LABEL_TENANT) != Some(&request.tenant_id)
        || container.labels.get(LABEL_LEASE) != Some(&request.execution_binding.lease_id)
        || container.labels.get(LABEL_LEASE_EPOCH)
            != Some(&request.execution_binding.lease_epoch.to_string())
        || container.labels.get(LABEL_SCOPE) != Some(&request.execution_binding.scope_digest)
    {
        return Err(ClosedError::new(
            ClosedReason::Conflict,
            "container ownership does not match the exact lease and scope",
        ));
    }
    Ok(())
}

fn validate_container_inspection_target(
    container: &Container,
    request: &DispatchRequestV1,
) -> Result<()> {
    validate_container_ownership(container, request)?;
    let Some(target) = request.payload.inspection_target() else {
        return Ok(());
    };
    let payload_digest = target.original_payload_digest.as_str();
    let allocation_attempt_drifted = target.original_effect_kind == "praxis.sandbox/allocate"
        && container.labels.get(LABEL_ATTEMPT).map(String::as_str)
            != Some(target.original_attempt_id.as_str());
    if allocation_attempt_drifted
        || container.labels.get(LABEL_PAYLOAD).map(String::as_str) != Some(payload_digest)
    {
        return Err(ClosedError::new(
            ClosedReason::Conflict,
            "container inspection target does not match the exact resource provenance",
        ));
    }
    Ok(())
}

fn container_payload(request: &DispatchRequestV1) -> Result<&ContainerPayloadV1> {
    let ProviderPayloadV1::ContainerdOci(payload) = &request.payload else {
        return Err(ClosedError::new(
            ClosedReason::BindingDrift,
            "non-container payload reached containerd provider",
        ));
    };
    Ok(payload)
}

fn container_id(request: &DispatchRequestV1) -> String {
    let mut digest = Sha256::new();
    digest.update(request.tenant_id.as_bytes());
    digest.update([0]);
    digest.update(request.execution_binding.lease_id.as_bytes());
    digest.update([0]);
    digest.update(request.execution_binding.lease_epoch.to_be_bytes());
    digest.update(request.execution_binding.instance_epoch.to_be_bytes());
    format!("praxis-{}", &hex::encode(digest.finalize())[..48])
}

fn namespaced<T>(message: T, namespace: &str) -> Result<Request<T>> {
    let metadata = MetadataValue::try_from(namespace).map_err(|_| {
        ClosedError::new(
            ClosedReason::InvalidArgument,
            "containerd namespace is invalid",
        )
    })?;
    let mut request = Request::new(message);
    request
        .metadata_mut()
        .insert("containerd-namespace", metadata);
    Ok(request)
}

fn path_to_string(path: &Path) -> Result<String> {
    path.to_str().map(ToOwned::to_owned).ok_or_else(|| {
        ClosedError::new(
            ClosedReason::InvalidArgument,
            "provider path is not valid UTF-8",
        )
    })
}

fn map_status(status: Status) -> ClosedError {
    let reason = match status.code() {
        Code::NotFound => ClosedReason::NotFoundObservation,
        Code::AlreadyExists | Code::Aborted | Code::FailedPrecondition => ClosedReason::Conflict,
        Code::PermissionDenied | Code::Unauthenticated => ClosedReason::UnauthorizedPeer,
        Code::ResourceExhausted => ClosedReason::ResourceLimit,
        Code::Unavailable | Code::DeadlineExceeded => ClosedReason::ProviderUnavailable,
        _ => ClosedReason::ProviderUnknown,
    };
    ClosedError::new(
        reason,
        format!("containerd request failed: {}", status.code()),
    )
}

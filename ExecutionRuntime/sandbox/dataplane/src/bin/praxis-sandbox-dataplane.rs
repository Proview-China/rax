use std::collections::BTreeMap;
use std::env;
use std::os::unix::fs::{FileTypeExt, PermissionsExt};
use std::path::{Path, PathBuf};
use std::sync::Arc;
use std::time::Duration;

use praxis_sandbox_dataplane::checkpoint::CheckpointStore;
use praxis_sandbox_dataplane::containerd::{ContainerdConfig, ContainerdProvider};
use praxis_sandbox_dataplane::contract::{
    DataPlaneOperationV1, DataPlaneRequestV1, DispatchResponseV1, ProviderKindV1,
};
use praxis_sandbox_dataplane::enforcer::DataPlaneEnforcer;
use praxis_sandbox_dataplane::error::{ClosedError, ClosedReason, Result};
use praxis_sandbox_dataplane::host::{HostConfig, HostProvider};
use praxis_sandbox_dataplane::ipc::{
    SocketCurrentFactsReader, read_frame, validate_peer, write_frame,
};
use praxis_sandbox_dataplane::journal::AttemptJournal;
use praxis_sandbox_dataplane::microvm::{MicroVmConfig, MicroVmProvider};
use praxis_sandbox_dataplane::remote::RemoteProvider;
use praxis_sandbox_dataplane::remote_ipc::SocketRemoteTransport;
use praxis_sandbox_dataplane::wasm::WasmProvider;
use praxis_sandbox_dataplane::wasm_capability_ipc::SocketWasmCapabilityHost;
use praxis_sandbox_dataplane::workspace_commit::{
    WorkspaceCommitConfigV1, WorkspaceCommitProviderV1,
};
use serde::Deserialize;
use tokio::fs;
use tokio::net::{UnixListener, UnixStream};

const ROOT_CONTRACT_VERSION: &str = "praxis.sandbox/data-plane-root/v1";

#[derive(Debug, Deserialize)]
#[serde(deny_unknown_fields)]
struct RootConfig {
    contract_version: String,
    dispatch_socket: PathBuf,
    current_reader_socket: PathBuf,
    journal_path: PathBuf,
    checkpoint_store_path: PathBuf,
    allowed_dispatch_uid: u32,
    allowed_current_reader_uid: u32,
    socket_mode: u32,
    #[serde(default)]
    host_workspace: Option<HostConfig>,
    #[serde(default)]
    qemu_microvm: Option<MicroVmConfig>,
    #[serde(default)]
    containerd: Option<ContainerdConfig>,
    #[serde(default)]
    wasmtime_component_bindings: BTreeMap<String, PathBuf>,
    #[serde(default)]
    wasmtime_capability_gateway_socket: Option<PathBuf>,
    #[serde(default)]
    allowed_wasmtime_capability_gateway_uid: Option<u32>,
    #[serde(default)]
    wasmtime_capability_gateway_timeout_millis: Option<u64>,
    #[serde(default)]
    remote_connector_socket: Option<PathBuf>,
    #[serde(default)]
    allowed_remote_connector_uid: Option<u32>,
    #[serde(default)]
    workspace_commit: Option<WorkspaceCommitConfigV1>,
}

struct RootState {
    enforcer: DataPlaneEnforcer<SocketCurrentFactsReader>,
    host: Option<HostProvider>,
    microvm: Option<MicroVmProvider>,
    containerd: Option<ContainerdProvider>,
    wasm: WasmProvider,
    remote: Option<RemoteProvider<SocketRemoteTransport>>,
    workspace_commit: Option<WorkspaceCommitProviderV1>,
    allowed_dispatch_uid: u32,
}

#[tokio::main]
async fn main() {
    tracing_subscriber::fmt()
        .with_target(false)
        .with_thread_ids(true)
        .init();
    if let Err(error) = run().await {
        tracing::error!(reason = ?error.reason, message = %error.message, "Sandbox Data Plane stopped");
        std::process::exit(1);
    }
}

async fn run() -> Result<()> {
    let config_path = env::args_os().nth(1).ok_or_else(|| {
        ClosedError::new(
            ClosedReason::InvalidArgument,
            "usage: praxis-sandbox-dataplane <config.json>",
        )
    })?;
    let bytes = fs::read(config_path).await.map_err(ClosedError::internal)?;
    let config: RootConfig = serde_json::from_slice(&bytes).map_err(|_| {
        ClosedError::new(
            ClosedReason::InvalidContract,
            "data plane root configuration is invalid",
        )
    })?;
    validate_root_config(&config)?;
    let journal = Arc::new(AttemptJournal::open(&config.journal_path).await?);
    let checkpoint = Arc::new(CheckpointStore::open(&config.checkpoint_store_path).await?);
    let reader = Arc::new(SocketCurrentFactsReader::new(
        &config.current_reader_socket,
        config.allowed_current_reader_uid,
    ));
    let mut wasm = WasmProvider::new(config.wasmtime_component_bindings);
    if let (Some(path), Some(uid), Some(timeout_millis)) = (
        config.wasmtime_capability_gateway_socket,
        config.allowed_wasmtime_capability_gateway_uid,
        config.wasmtime_capability_gateway_timeout_millis,
    ) {
        wasm = wasm.with_capability_host(Arc::new(SocketWasmCapabilityHost::new(
            path,
            uid,
            Duration::from_millis(timeout_millis),
        )?));
    }
    let remote = match (
        config.remote_connector_socket,
        config.allowed_remote_connector_uid,
    ) {
        (Some(path), Some(uid)) => {
            Some(RemoteProvider::new(SocketRemoteTransport::new(path, uid)?))
        }
        (None, None) => None,
        _ => unreachable!("validated remote connector presence"),
    };
    let host = config.host_workspace.map(HostProvider::new);
    if let Some(provider) = &host {
        provider.probe().await?;
    }
    let microvm = config.qemu_microvm.map(MicroVmProvider::new);
    if let Some(provider) = &microvm {
        provider.probe().await?;
    }
    let containerd = config.containerd.map(ContainerdProvider::new);
    if let Some(provider) = &containerd {
        provider.probe().await?;
    }
    let workspace_commit = config.workspace_commit.map(WorkspaceCommitProviderV1::new);
    if let Some(provider) = &workspace_commit {
        provider.probe().await?;
    }
    let state = Arc::new(RootState {
        enforcer: DataPlaneEnforcer::new(reader, journal).with_checkpoint_store(checkpoint),
        host,
        microvm,
        containerd,
        wasm,
        remote,
        workspace_commit,
        allowed_dispatch_uid: config.allowed_dispatch_uid,
    });
    let listener = bind_socket(&config.dispatch_socket, config.socket_mode).await?;
    tracing::info!(socket = %config.dispatch_socket.display(), "Sandbox Data Plane ready");
    loop {
        let (stream, _) = listener.accept().await.map_err(ClosedError::internal)?;
        let state = Arc::clone(&state);
        tokio::spawn(async move {
            if let Err(error) = serve_connection(state, stream).await {
                tracing::warn!(reason = ?error.reason, "dispatch connection rejected");
            }
        });
    }
}

async fn serve_connection(state: Arc<RootState>, mut stream: UnixStream) -> Result<()> {
    validate_peer(&stream, state.allowed_dispatch_uid)?;
    let envelope: DataPlaneRequestV1 = read_frame(&mut stream).await?;
    envelope.validate(praxis_sandbox_dataplane::contract::now_unix_nano())?;
    let request = &envelope.request;
    let result = match envelope.operation {
        DataPlaneOperationV1::Inspect => state.enforcer.inspect(request).await,
        DataPlaneOperationV1::Dispatch => match request.payload.kind() {
            ProviderKindV1::HostWorkspace => match &state.host {
                Some(provider) => state.enforcer.dispatch(provider, request).await,
                None => Err(provider_not_configured("Host Workspace")),
            },
            ProviderKindV1::QemuMicrovm => match &state.microvm {
                Some(provider) => state.enforcer.dispatch(provider, request).await,
                None => Err(provider_not_configured("QEMU/KVM MicroVM")),
            },
            ProviderKindV1::ContainerdOci => match &state.containerd {
                Some(provider) => state.enforcer.dispatch(provider, request).await,
                None => Err(provider_not_configured("containerd/OCI")),
            },
            ProviderKindV1::WasmtimeComponent => {
                state.enforcer.dispatch(&state.wasm, request).await
            }
            ProviderKindV1::RemoteSandbox => match &state.remote {
                Some(provider) => state.enforcer.dispatch(provider, request).await,
                None => Err(ClosedError::new(
                    ClosedReason::Unsupported,
                    "remote transport is not configured in this root",
                )),
            },
            ProviderKindV1::WorkspaceCommit => match &state.workspace_commit {
                Some(provider) => state.enforcer.dispatch(provider, request).await,
                None => Err(ClosedError::new(
                    ClosedReason::Unsupported,
                    "workspace commit Provider is not configured in this root",
                )),
            },
        },
    };
    let response = match result {
        Ok(result) => DispatchResponseV1::success(request, &result)?,
        Err(error) => DispatchResponseV1::failure(request, error)?,
    };
    write_frame(&mut stream, &response).await
}

fn provider_not_configured(name: &str) -> ClosedError {
    ClosedError::new(
        ClosedReason::Unsupported,
        format!("{name} Provider is not configured in this root"),
    )
}

fn validate_root_config(config: &RootConfig) -> Result<()> {
    let capability_presence = (
        config.wasmtime_capability_gateway_socket.is_some(),
        config.allowed_wasmtime_capability_gateway_uid.is_some(),
        config.wasmtime_capability_gateway_timeout_millis.is_some(),
    );
    let remote_presence = (
        config.remote_connector_socket.is_some(),
        config.allowed_remote_connector_uid.is_some(),
    );
    if config.contract_version != ROOT_CONTRACT_VERSION
        || config.dispatch_socket.as_os_str().is_empty()
        || config.current_reader_socket.as_os_str().is_empty()
        || config.journal_path.as_os_str().is_empty()
        || config.checkpoint_store_path.as_os_str().is_empty()
        || !config.checkpoint_store_path.is_absolute()
        || config.dispatch_socket == config.current_reader_socket
        || config.socket_mode & !0o777 != 0
        || config.socket_mode & 0o007 != 0
        || !matches!(
            capability_presence,
            (false, false, false) | (true, true, true)
        )
        || !matches!(remote_presence, (false, false) | (true, true))
        || config
            .wasmtime_capability_gateway_timeout_millis
            .is_some_and(|value| value == 0 || value > 30_000)
    {
        return Err(ClosedError::new(
            ClosedReason::InvalidContract,
            "data plane root configuration violates the closed policy",
        ));
    }
    Ok(())
}

async fn bind_socket(path: &Path, mode: u32) -> Result<UnixListener> {
    if let Some(parent) = path.parent() {
        fs::create_dir_all(parent)
            .await
            .map_err(ClosedError::internal)?;
    }
    match fs::symlink_metadata(path).await {
        Ok(metadata) if metadata.file_type().is_socket() => {
            fs::remove_file(path).await.map_err(ClosedError::internal)?;
        }
        Ok(_) => {
            return Err(ClosedError::new(
                ClosedReason::Conflict,
                "dispatch socket path is occupied by a non-socket",
            ));
        }
        Err(error) if error.kind() == std::io::ErrorKind::NotFound => {}
        Err(error) => return Err(ClosedError::internal(error)),
    }
    let listener = UnixListener::bind(path).map_err(ClosedError::internal)?;
    fs::set_permissions(path, std::fs::Permissions::from_mode(mode))
        .await
        .map_err(ClosedError::internal)?;
    Ok(listener)
}

#[cfg(test)]
mod tests {
    use super::*;

    fn root_config_json(extra: &str) -> String {
        format!(
            r#"{{
                "contract_version":"praxis.sandbox/data-plane-root/v1",
                "dispatch_socket":"/tmp/praxis-sandbox-dispatch.sock",
                "current_reader_socket":"/tmp/praxis-sandbox-current.sock",
                "journal_path":"/tmp/praxis-sandbox-journal.sqlite",
                "checkpoint_store_path":"/tmp/praxis-sandbox-checkpoints.sqlite",
                "allowed_dispatch_uid":1000,
                "allowed_current_reader_uid":1000,
                "socket_mode":432,
                "wasmtime_component_bindings":{{}}{extra}
            }}"#
        )
    }

    #[test]
    fn root_accepts_only_configured_backends() {
        let config: RootConfig = serde_json::from_str(&root_config_json(""))
            .unwrap_or_else(|error| panic!("parse root config: {error}"));
        assert!(config.host_workspace.is_none());
        assert!(config.qemu_microvm.is_none());
        assert!(config.containerd.is_none());
        validate_root_config(&config)
            .unwrap_or_else(|error| panic!("validate root config: {error}"));
    }

    #[test]
    fn root_rejects_partial_capability_gateway_binding() {
        let config: RootConfig = serde_json::from_str(&root_config_json(
            r#", "wasmtime_capability_gateway_socket":"/tmp/capability.sock""#,
        ))
        .unwrap_or_else(|error| panic!("parse partial gateway config: {error}"));
        assert!(validate_root_config(&config).is_err());
    }
}

mod common;

use std::collections::BTreeMap;
use std::fs;
use std::os::unix::fs::PermissionsExt;
use std::path::PathBuf;
use std::time::Duration;

use praxis_sandbox_dataplane::containerd::{ContainerdConfig, ContainerdProvider, RootfsBinding};
use praxis_sandbox_dataplane::contract::{
    ContainerPayloadV1, DispatchRequestV1, EnforcementPhaseV1, ProviderInspectionTargetV1,
    ProviderPayloadV1,
};
use praxis_sandbox_dataplane::provider::Provider;

#[tokio::test]
#[ignore = "requires an active privileged local containerd"]
#[allow(clippy::too_many_lines)]
async fn live_allocate_activate_inspect_and_release() {
    let temporary = tempfile::tempdir().unwrap_or_else(|error| panic!("tempdir: {error}"));
    let lease_id = format!(
        "lease-live-{}",
        temporary
            .path()
            .file_name()
            .and_then(|value| value.to_str())
            .unwrap_or("containerd")
    );
    let rootfs = temporary.path().join("rootfs");
    fs::create_dir_all(rootfs.join("bin")).unwrap_or_else(|error| panic!("create rootfs: {error}"));
    fs::copy("/usr/bin/busybox", rootfs.join("bin/busybox"))
        .unwrap_or_else(|error| panic!("copy busybox: {error}"));
    fs::set_permissions(
        rootfs.join("bin/busybox"),
        fs::Permissions::from_mode(0o755),
    )
    .unwrap_or_else(|error| panic!("chmod busybox: {error}"));
    std::os::unix::fs::symlink("busybox", rootfs.join("bin/sh"))
        .unwrap_or_else(|error| panic!("link shell: {error}"));

    let image_digest = common::digest("busybox-rootfs");
    let payload = ProviderPayloadV1::ContainerdOci(ContainerPayloadV1 {
        rootfs_binding_id: "rootfs-1".to_owned(),
        image_digest: image_digest.clone(),
        argv: vec![
            "/bin/sh".to_owned(),
            "-c".to_owned(),
            "echo praxis-container-backend".to_owned(),
        ],
        environment: BTreeMap::new(),
        working_directory: "/".to_owned(),
        read_only_rootfs: true,
        cpu_quota_micros: 50_000,
        memory_limit_bytes: 64 * 1024 * 1024,
        pids_limit: 32,
        network_deny_all: true,
        inspection_target: None,
    });
    let provider = ContainerdProvider::new(ContainerdConfig {
        socket_path: PathBuf::from("/run/containerd/containerd.sock"),
        namespace: "default".to_owned(),
        runtime_name: "io.containerd.runc.v2".to_owned(),
        state_directory: temporary.path().join("state"),
        rootfs_bindings: BTreeMap::from([(
            "rootfs-1".to_owned(),
            RootfsBinding {
                path: rootfs,
                image_digest,
            },
        )]),
    });

    provider
        .probe()
        .await
        .unwrap_or_else(|error| panic!("containerd probe: {error}"));
    let allocate_prepare = effect_request(
        payload.clone(),
        "praxis.sandbox/allocate",
        "allocate-1",
        EnforcementPhaseV1::Prepare,
        &lease_id,
    );
    provider
        .prepare(&allocate_prepare)
        .await
        .unwrap_or_else(|error| panic!("allocate prepare: {error}"));
    let allocate_execute = effect_request(
        payload.clone(),
        "praxis.sandbox/allocate",
        "allocate-1",
        EnforcementPhaseV1::Execute,
        &lease_id,
    );
    provider
        .execute_prepared(&allocate_execute)
        .await
        .unwrap_or_else(|error| panic!("allocate execute: {error}"));

    let activate_prepare = effect_request(
        payload.clone(),
        "praxis.sandbox/activate",
        "activate-1",
        EnforcementPhaseV1::Prepare,
        &lease_id,
    );
    provider
        .prepare(&activate_prepare)
        .await
        .unwrap_or_else(|error| panic!("activate prepare: {error}"));
    let activate_execute = effect_request(
        payload.clone(),
        "praxis.sandbox/activate",
        "activate-1",
        EnforcementPhaseV1::Execute,
        &lease_id,
    );
    let activated = provider
        .execute_prepared(&activate_execute)
        .await
        .unwrap_or_else(|error| panic!("activate execute: {error}"));

    let ProviderPayloadV1::ContainerdOci(mut inspect_container) = payload.clone() else {
        unreachable!("container fixture uses a container payload")
    };
    inspect_container.inspection_target = Some(ProviderInspectionTargetV1 {
        original_effect_kind: "praxis.sandbox/activate".to_owned(),
        original_attempt_id: activate_execute.attempt_id.clone(),
        provider_attempt: activated.attempt,
        original_request_digest: activate_execute.digest.clone(),
        original_payload_digest: activate_execute.payload_digest.clone(),
    });
    let inspect = effect_request(
        ProviderPayloadV1::ContainerdOci(inspect_container),
        "praxis.sandbox/inspect",
        "inspect-1",
        EnforcementPhaseV1::Execute,
        &lease_id,
    );
    let mut stopped = false;
    for _ in 0..50 {
        let result = provider
            .inspect(&inspect)
            .await
            .unwrap_or_else(|error| panic!("inspect: {error}"));
        if result.observation.state.starts_with("task:stopped") {
            stopped = true;
            break;
        }
        tokio::time::sleep(Duration::from_millis(20)).await;
    }
    assert!(stopped, "container task did not reach stopped state");

    let release = effect_request(
        payload,
        "praxis.sandbox/release",
        "release-1",
        EnforcementPhaseV1::Execute,
        &lease_id,
    );
    provider
        .cleanup(&release)
        .await
        .unwrap_or_else(|error| panic!("cleanup stopped task: {error}"));
    let cleaned = provider
        .inspect_cleanup(&release)
        .await
        .unwrap_or_else(|error| panic!("cleanup inspect: {error}"));
    assert_eq!(cleaned.observation.state, "cleanup_absent");
}

fn effect_request(
    payload: ProviderPayloadV1,
    effect_kind: &str,
    attempt_id: &str,
    phase: EnforcementPhaseV1,
    lease_id: &str,
) -> DispatchRequestV1 {
    let bootstrap_payload = match &payload {
        ProviderPayloadV1::HostWorkspace(value) => {
            let mut value = value.clone();
            value.inspection_target = None;
            ProviderPayloadV1::HostWorkspace(value)
        }
        ProviderPayloadV1::QemuMicrovm(value) => {
            let mut value = value.clone();
            value.inspection_target = None;
            ProviderPayloadV1::QemuMicrovm(value)
        }
        ProviderPayloadV1::ContainerdOci(value) => {
            let mut value = value.clone();
            value.inspection_target = None;
            ProviderPayloadV1::ContainerdOci(value)
        }
        ProviderPayloadV1::WasmtimeComponent(value) => {
            let mut value = value.clone();
            value.inspection_target = None;
            ProviderPayloadV1::WasmtimeComponent(value)
        }
        ProviderPayloadV1::RemoteSandbox(value) => {
            let mut value = value.clone();
            value.inspection_target = None;
            ProviderPayloadV1::RemoteSandbox(value)
        }
        ProviderPayloadV1::WorkspaceCommit(value) => {
            let mut value = value.clone();
            value.inspection_target = None;
            ProviderPayloadV1::WorkspaceCommit(value)
        }
    };
    let mut request = common::request_with_payload(phase, bootstrap_payload);
    request.payload = payload;
    request.request_id = format!("{attempt_id}-{phase:?}");
    request.effect_kind = effect_kind.to_owned();
    request.effect_id = format!("effect-{attempt_id}");
    request.attempt_id = attempt_id.to_owned();
    request.sandbox_attempt.id = attempt_id.to_owned();
    request.sandbox_attempt.digest = common::digest(attempt_id);
    request.execution_binding.lease_id = lease_id.to_owned();
    request.runtime_enforcement.effect_id = request.effect_id.clone();
    request.runtime_enforcement.attempt_id = attempt_id.to_owned();
    request.runtime_enforcement.phase = phase;
    request.runtime_enforcement.receipt_digest = common::digest(&request.request_id);
    request
        .seal()
        .unwrap_or_else(|error| panic!("effect request seal: {error}"))
}

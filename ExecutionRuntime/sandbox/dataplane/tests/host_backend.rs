mod common;

use std::collections::{BTreeMap, BTreeSet};
use std::fs;
use std::path::{Path, PathBuf};
use std::sync::Arc;
use std::time::Duration;

use praxis_sandbox_dataplane::contract::{
    DispatchRequestV1, EnforcementPhaseV1, HostWorkspacePayloadV1, ProviderInspectionTargetV1,
    ProviderPayloadV1,
};
use praxis_sandbox_dataplane::error::ClosedReason;
use praxis_sandbox_dataplane::host::{
    HostConfig, HostProvider, HostToolBinding, HostWorkspaceBinding,
};
use praxis_sandbox_dataplane::provider::Provider;
use sha2::{Digest as _, Sha256};

#[tokio::test]
async fn real_bwrap_workspace_execute_inspect_and_cleanup() {
    let fixture = Fixture::new();
    let provider = fixture.provider();
    provider
        .probe()
        .await
        .unwrap_or_else(|error| panic!("probe: {error}"));

    let allocate = fixture.request(
        "praxis.sandbox/allocate",
        "allocate-1",
        EnforcementPhaseV1::Execute,
        "printf praxis-host > result; test ! -e /home/proview",
    );
    let allocated = provider
        .execute_prepared(&allocate)
        .await
        .unwrap_or_else(|error| panic!("allocate: {error}"));
    assert_eq!(allocated.observation.state, "workspace_allocated");

    let activate_prepare = fixture.request(
        "praxis.sandbox/activate",
        "activate-1",
        EnforcementPhaseV1::Prepare,
        "printf praxis-host > result; test ! -e /home/proview",
    );
    provider
        .prepare(&activate_prepare)
        .await
        .unwrap_or_else(|error| panic!("activate prepare: {error}"));
    let activate = fixture.request(
        "praxis.sandbox/activate",
        "activate-1",
        EnforcementPhaseV1::Execute,
        "printf praxis-host > result; test ! -e /home/proview",
    );
    let executed = provider
        .execute_prepared(&activate)
        .await
        .unwrap_or_else(|error| panic!("activate execute: {error}"));
    assert_eq!(executed.observation.state, "exited:0");
    assert_eq!(
        fs::read_to_string(fixture.workspace.join("work/result"))
            .unwrap_or_else(|error| panic!("read result: {error}")),
        "praxis-host"
    );

    let inspect = fixture.inspect_request(&activate, executed.attempt);
    let inspected = provider
        .inspect(&inspect)
        .await
        .unwrap_or_else(|error| panic!("inspect: {error}"));
    assert_eq!(inspected.observation.state, "exited:0");

    let cleanup = fixture.request(
        "praxis.sandbox/cleanup",
        "cleanup-1",
        EnforcementPhaseV1::Execute,
        "true",
    );
    let cleaned = provider
        .cleanup(&cleanup)
        .await
        .unwrap_or_else(|error| panic!("cleanup: {error}"));
    assert_eq!(cleaned.observation.state, "cleanup_absent");
}

#[tokio::test]
async fn live_process_requires_independent_fence_before_release() {
    let fixture = Fixture::new();
    let provider = Arc::new(fixture.provider());
    let allocate = fixture.request(
        "praxis.sandbox/allocate",
        "allocate-2",
        EnforcementPhaseV1::Execute,
        "sleep 30",
    );
    provider
        .execute_prepared(&allocate)
        .await
        .unwrap_or_else(|error| panic!("allocate: {error}"));
    let activate = fixture.request(
        "praxis.sandbox/activate",
        "activate-2",
        EnforcementPhaseV1::Execute,
        "sleep 30",
    );
    let observe_running = activate.clone();
    let executing_provider = Arc::clone(&provider);
    let executing =
        tokio::spawn(async move { executing_provider.execute_prepared(&activate).await });

    let mut running = false;
    for _ in 0..100 {
        if provider
            .inspect(&observe_running)
            .await
            .is_ok_and(|result| result.observation.state == "running")
        {
            running = true;
            break;
        }
        tokio::time::sleep(Duration::from_millis(10)).await;
    }
    assert!(running, "host process did not become inspectably running");

    let release = fixture.request(
        "praxis.sandbox/release",
        "release-2",
        EnforcementPhaseV1::Execute,
        "true",
    );
    let release_error = common::must_error(provider.release(&release).await);
    assert_eq!(release_error.reason, ClosedReason::Conflict);

    let fence = fixture.request(
        "praxis.sandbox/fence",
        "fence-2",
        EnforcementPhaseV1::Execute,
        "true",
    );
    let fenced = provider
        .fence(&fence)
        .await
        .unwrap_or_else(|error| panic!("fence: {error}"));
    assert_eq!(fenced.observation.state, "fence_signal_sent");
    let execution = executing
        .await
        .unwrap_or_else(|error| panic!("join execute: {error}"))
        .unwrap_or_else(|error| panic!("execute after fence: {error}"));
    assert_eq!(execution.observation.state, "exited:signal");
    provider
        .release(&release)
        .await
        .unwrap_or_else(|error| panic!("release after fence: {error}"));
}

#[tokio::test]
async fn binding_digest_and_symlink_escape_fail_closed() {
    let fixture = Fixture::new();
    let provider = fixture.provider();
    let mut wrong = fixture.request(
        "praxis.sandbox/allocate",
        "allocate-wrong",
        EnforcementPhaseV1::Prepare,
        "true",
    );
    let ProviderPayloadV1::HostWorkspace(mut payload) = wrong.payload.clone() else {
        panic!("host fixture payload")
    };
    payload.tool_digest = common::digest("wrong-tool");
    wrong.payload = ProviderPayloadV1::HostWorkspace(payload);
    wrong = wrong
        .seal()
        .unwrap_or_else(|error| panic!("reseal wrong payload: {error}"));
    let error = common::must_error(provider.prepare(&wrong).await);
    assert_eq!(error.reason, ClosedReason::InvalidDigest);

    let link = fixture.root.path().join("workspace-link");
    std::os::unix::fs::symlink(&fixture.workspace, &link)
        .unwrap_or_else(|error| panic!("symlink: {error}"));
    let mut config = fixture.config();
    config
        .workspace_bindings
        .get_mut("workspace-1")
        .unwrap_or_else(|| panic!("workspace binding"))
        .path = link;
    let linked = HostProvider::new(config);
    let request = fixture.request(
        "praxis.sandbox/allocate",
        "allocate-link",
        EnforcementPhaseV1::Prepare,
        "true",
    );
    let error = common::must_error(linked.prepare(&request).await);
    assert_eq!(error.reason, ClosedReason::InvalidContract);
}

struct Fixture {
    root: tempfile::TempDir,
    workspace: PathBuf,
    tool_digest: String,
}

impl Fixture {
    fn new() -> Self {
        let root = tempfile::tempdir().unwrap_or_else(|error| panic!("tempdir: {error}"));
        let workspace = root.path().join("workspace");
        fs::create_dir_all(workspace.join("work"))
            .unwrap_or_else(|error| panic!("workspace: {error}"));
        Self {
            root,
            workspace,
            tool_digest: file_digest(Path::new("/usr/bin/dash")),
        }
    }

    fn config(&self) -> HostConfig {
        HostConfig {
            bwrap_path: PathBuf::from("/usr/bin/bwrap"),
            state_directory: self.root.path().join("state"),
            runtime_readonly_paths: vec![PathBuf::from("/usr")],
            allowed_environment_keys: BTreeSet::from(["LANG".to_owned()]),
            workspace_bindings: BTreeMap::from([(
                "workspace-1".to_owned(),
                HostWorkspaceBinding {
                    path: self.workspace.clone(),
                    digest: common::digest("workspace-1"),
                    writable_overlay: true,
                },
            )]),
            tool_bindings: BTreeMap::from([(
                "tool-1".to_owned(),
                HostToolBinding {
                    path: PathBuf::from("/usr/bin/dash"),
                    digest: self.tool_digest.clone(),
                },
            )]),
        }
    }

    fn provider(&self) -> HostProvider {
        HostProvider::new(self.config())
    }

    fn payload(&self, command: &str) -> ProviderPayloadV1 {
        ProviderPayloadV1::HostWorkspace(HostWorkspacePayloadV1 {
            workspace_binding_id: "workspace-1".to_owned(),
            workspace_digest: common::digest("workspace-1"),
            tool_binding_id: "tool-1".to_owned(),
            tool_digest: self.tool_digest.clone(),
            argv: vec!["-c".to_owned(), command.to_owned()],
            environment: BTreeMap::new(),
            working_directory: "work".to_owned(),
            network_deny_all: true,
            wall_clock_timeout_millis: 60_000,
            inspection_target: None,
        })
    }

    fn request(
        &self,
        effect_kind: &str,
        attempt_id: &str,
        phase: EnforcementPhaseV1,
        command: &str,
    ) -> DispatchRequestV1 {
        let mut request = common::request_with_payload(phase, self.payload(command));
        request.request_id = format!("{attempt_id}-{phase:?}");
        request.effect_kind = effect_kind.to_owned();
        request.effect_id = format!("effect-{attempt_id}");
        request.attempt_id = attempt_id.to_owned();
        request.sandbox_attempt.id = attempt_id.to_owned();
        request.sandbox_attempt.digest = common::digest(attempt_id);
        request
            .runtime_enforcement
            .effect_id
            .clone_from(&request.effect_id);
        request.runtime_enforcement.attempt_id = attempt_id.to_owned();
        request.runtime_enforcement.receipt_digest = common::digest(&request.request_id);
        request
            .seal()
            .unwrap_or_else(|error| panic!("host request: {error}"))
    }

    fn inspect_request(
        &self,
        original: &DispatchRequestV1,
        provider_attempt: praxis_sandbox_dataplane::contract::ExactRefV1,
    ) -> DispatchRequestV1 {
        let ProviderPayloadV1::HostWorkspace(mut payload) = original.payload.clone() else {
            panic!("host original payload")
        };
        payload.inspection_target = Some(ProviderInspectionTargetV1 {
            original_effect_kind: original.effect_kind.clone(),
            original_attempt_id: original.attempt_id.clone(),
            provider_attempt,
            original_request_digest: original.digest.clone(),
            original_payload_digest: original.payload_digest.clone(),
        });
        let mut request = self.request(
            "praxis.sandbox/open",
            "inspect-1",
            EnforcementPhaseV1::Execute,
            "true",
        );
        request.effect_kind = "praxis.sandbox/inspect".to_owned();
        request.payload = ProviderPayloadV1::HostWorkspace(payload);
        request
            .seal()
            .unwrap_or_else(|error| panic!("host inspect: {error}"))
    }
}

fn file_digest(path: &Path) -> String {
    let bytes = fs::read(path).unwrap_or_else(|error| panic!("read tool: {error}"));
    format!("sha256:{}", hex::encode(Sha256::digest(bytes)))
}

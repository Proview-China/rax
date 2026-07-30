#![allow(clippy::expect_used, clippy::unwrap_used)]

mod common;

use std::collections::BTreeMap;
use std::fs;
use std::sync::Arc;

use praxis_sandbox_dataplane::contract::{
    EnforcementPhaseV1, ProviderPayloadV1, WorkspaceReadPayloadV1, now_unix_nano,
};
use praxis_sandbox_dataplane::enforcer::DataPlaneEnforcer;
use praxis_sandbox_dataplane::error::{ClosedReason, EffectBoundary};
use praxis_sandbox_dataplane::journal::AttemptJournal;
use praxis_sandbox_dataplane::workspace_read::{
    WorkspaceReadBindingV1, WorkspaceReadConfigV1, WorkspaceReadProviderV1,
};

fn workspace_request(
    phase: EnforcementPhaseV1,
    workspace: &praxis_sandbox_dataplane::contract::ExactRefV1,
    start_byte: u64,
    max_bytes: u64,
) -> praxis_sandbox_dataplane::contract::DispatchRequestV1 {
    let payload = ProviderPayloadV1::WorkspaceRead(WorkspaceReadPayloadV1 {
        workspace_binding_id: "workspace-1".to_owned(),
        workspace_digest: common::digest("workspace-binding"),
        workspace: workspace.clone(),
        file_scope_digest: common::digest("file-scope"),
        relative_path: "src/main.txt".to_owned(),
        start_byte,
        max_bytes,
        expected_file_ref: None,
        s1_checked: true,
        inspection_target: None,
    });
    let mut request = common::request_with_payload(phase, payload);
    request.effect_kind = "praxis.sandbox/workspace-read".to_owned();
    request
        .seal()
        .unwrap_or_else(|error| panic!("seal workspace read: {error}"))
}

fn provider(root: &std::path::Path) -> WorkspaceReadProviderV1 {
    WorkspaceReadProviderV1::new(WorkspaceReadConfigV1 {
        bindings: BTreeMap::from([(
            "workspace-1".to_owned(),
            WorkspaceReadBindingV1 {
                path: root.to_path_buf(),
                digest: common::digest("workspace-binding"),
            },
        )]),
    })
}

#[tokio::test]
async fn sixty_four_replays_cross_the_physical_read_once_and_restart_inspects() {
    let temporary = tempfile::tempdir().unwrap_or_else(|error| panic!("tempdir: {error}"));
    fs::create_dir_all(temporary.path().join("src"))
        .unwrap_or_else(|error| panic!("mkdir: {error}"));
    fs::write(temporary.path().join("src/main.txt"), "hello Praxis")
        .unwrap_or_else(|error| panic!("write: {error}"));
    let workspace = common::exact("workspace-1", now_unix_nano() + 60_000_000_000);
    let prepare = workspace_request(EnforcementPhaseV1::Prepare, &workspace, 6, 6);
    let execute = workspace_request(EnforcementPhaseV1::Execute, &workspace, 6, 6);
    let journal_path = temporary.path().join("attempts.jsonl");
    let journal = Arc::new(
        AttemptJournal::open(&journal_path)
            .await
            .unwrap_or_else(|error| panic!("journal: {error}")),
    );
    let enforcer = Arc::new(DataPlaneEnforcer::new(common::reader(), journal));
    let provider = Arc::new(provider(temporary.path()));
    enforcer
        .dispatch(provider.as_ref(), &prepare)
        .await
        .unwrap_or_else(|error| panic!("prepare: {error}"));

    let mut tasks = Vec::new();
    for _ in 0..64 {
        let enforcer = Arc::clone(&enforcer);
        let provider = Arc::clone(&provider);
        let execute = execute.clone();
        tasks.push(tokio::spawn(async move {
            enforcer.dispatch(provider.as_ref(), &execute).await
        }));
    }
    let mut successes = Vec::new();
    for task in tasks {
        if let Ok(value) = task.await.unwrap_or_else(|error| panic!("join: {error}")) {
            successes.push(value);
        }
    }
    assert_eq!(successes.len(), 1);
    assert_eq!(provider.physical_read_count(), 1);
    let observation = successes[0]
        .observation
        .workspace_read
        .as_ref()
        .unwrap_or_else(|| panic!("workspace observation"));
    assert_eq!(observation.content, "Praxis");
    assert_eq!(observation.start_byte, 6);
    assert_eq!(observation.returned_bytes, 6);
    assert!(observation.complete);

    drop(enforcer);
    let reopened = Arc::new(
        AttemptJournal::open(&journal_path)
            .await
            .unwrap_or_else(|error| panic!("reopen journal: {error}")),
    );
    let recovered = DataPlaneEnforcer::new(common::reader(), reopened)
        .inspect(&execute)
        .await
        .unwrap_or_else(|error| panic!("inspect exact result: {error}"));
    assert_eq!(recovered, successes[0]);
    assert_eq!(provider.physical_read_count(), 1);
}

#[tokio::test]
async fn started_then_restart_is_indeterminate_and_does_not_read() {
    let temporary = tempfile::tempdir().unwrap_or_else(|error| panic!("tempdir: {error}"));
    fs::create_dir_all(temporary.path().join("src"))
        .unwrap_or_else(|error| panic!("mkdir: {error}"));
    fs::write(temporary.path().join("src/main.txt"), "stable")
        .unwrap_or_else(|error| panic!("write: {error}"));
    let workspace = common::exact("workspace-1", now_unix_nano() + 60_000_000_000);
    let prepare = workspace_request(EnforcementPhaseV1::Prepare, &workspace, 0, 64);
    let execute = workspace_request(EnforcementPhaseV1::Execute, &workspace, 0, 64);
    let path = temporary.path().join("started.jsonl");
    let journal = Arc::new(AttemptJournal::open(&path).await.unwrap());
    let provider = provider(temporary.path());
    let enforcer = DataPlaneEnforcer::new(common::reader(), Arc::clone(&journal));
    enforcer.dispatch(&provider, &prepare).await.unwrap();
    journal.begin(&execute).await.unwrap();
    drop(enforcer);
    drop(journal);

    let reopened = Arc::new(AttemptJournal::open(&path).await.unwrap());
    let error = DataPlaneEnforcer::new(common::reader(), reopened)
        .inspect(&execute)
        .await
        .expect_err("started attempt must remain unknown");
    assert_eq!(error.reason, ClosedReason::ProviderUnknown);
    assert_eq!(provider.physical_read_count(), 0);
}

#[tokio::test]
async fn rejects_symlink_non_utf8_and_oversized_sources() {
    let temporary = tempfile::tempdir().unwrap();
    fs::create_dir_all(temporary.path().join("src")).unwrap();
    let outside = temporary.path().join("outside.txt");
    fs::write(&outside, "outside").unwrap();
    std::os::unix::fs::symlink(&outside, temporary.path().join("src/main.txt")).unwrap();
    let workspace = common::exact("workspace-1", now_unix_nano() + 60_000_000_000);
    let request = workspace_request(EnforcementPhaseV1::Execute, &workspace, 0, 64);
    let provider = provider(temporary.path());
    let error = praxis_sandbox_dataplane::provider::Provider::execute_prepared(&provider, &request)
        .await
        .expect_err("symlink must fail closed");
    assert!(matches!(
        error.reason,
        ClosedReason::BindingDrift | ClosedReason::ProviderUnavailable
    ));
    assert_eq!(
        error.effect_boundary,
        Some(EffectBoundary::EffectNotStarted)
    );
    assert_eq!(error.crossed_actual_point, Some(false));
    assert_eq!(error.physical_read_count, Some(0));
    assert_eq!(provider.physical_read_count(), 0);

    fs::remove_file(temporary.path().join("src/main.txt")).unwrap();
    fs::write(temporary.path().join("src/main.txt"), [0xff, 0xfe]).unwrap();
    let error = praxis_sandbox_dataplane::provider::Provider::execute_prepared(&provider, &request)
        .await
        .expect_err("non UTF-8 must fail closed");
    assert_eq!(error.reason, ClosedReason::InvalidArgument);
    assert_eq!(
        error.effect_boundary,
        Some(EffectBoundary::EffectStartedUnknown)
    );
    assert_eq!(error.crossed_actual_point, Some(true));
    assert_eq!(error.physical_read_count, Some(1));
    assert_eq!(provider.physical_read_count(), 1);

    fs::write(temporary.path().join("src/main.txt"), vec![b'x'; 1_048_577]).unwrap();
    let error = praxis_sandbox_dataplane::provider::Provider::execute_prepared(&provider, &request)
        .await
        .expect_err("oversized file must fail closed");
    assert_eq!(error.reason, ClosedReason::FrameTooLarge);
    assert_eq!(
        error.effect_boundary,
        Some(EffectBoundary::EffectNotStarted)
    );
    assert_eq!(error.crossed_actual_point, Some(false));
    assert_eq!(error.physical_read_count, Some(0));
    assert_eq!(provider.physical_read_count(), 1);

    fs::write(temporary.path().join("src/main.txt"), "tiny").unwrap();
    let beyond_eof = workspace_request(EnforcementPhaseV1::Execute, &workspace, 5, 64);
    let error =
        praxis_sandbox_dataplane::provider::Provider::execute_prepared(&provider, &beyond_eof)
            .await
            .expect_err("start beyond EOF must fail before the actual point");
    assert_eq!(error.reason, ClosedReason::InvalidArgument);
    assert_eq!(
        error.effect_boundary,
        Some(EffectBoundary::EffectNotStarted)
    );
    assert_eq!(error.crossed_actual_point, Some(false));
    assert_eq!(error.physical_read_count, Some(0));
    assert_eq!(provider.physical_read_count(), 1);
}

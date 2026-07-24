mod common;

use std::collections::BTreeMap;
use std::fs;
use std::os::unix::fs::symlink;
use std::sync::Arc;

use praxis_sandbox_dataplane::contract::{
    DispatchRequestV1, EnforcementPhaseV1, ExactRefV1, ProviderInspectionTargetV1,
    ProviderPayloadV1, WorkspaceCommitPayloadV1, WorkspaceMutationV1, canonical_digest,
    now_unix_nano,
};
use praxis_sandbox_dataplane::enforcer::DataPlaneEnforcer;
use praxis_sandbox_dataplane::error::ClosedReason;
use praxis_sandbox_dataplane::journal::AttemptJournal;
use praxis_sandbox_dataplane::provider::Provider;
use praxis_sandbox_dataplane::workspace_commit::{
    WorkspaceCommitBindingV1, WorkspaceCommitConfigV1, WorkspaceCommitProviderV1,
};
use serde::Serialize;
use sha2::{Digest as _, Sha256};

#[derive(Serialize)]
struct FixtureTreeEntry {
    path: String,
    mode: u32,
    digest: String,
    length: usize,
}

#[derive(Serialize)]
struct FixtureBlobDescriptor {
    content_digest: String,
    length: usize,
    mode: u32,
}

struct Fixture {
    root: tempfile::TempDir,
    workspace: std::path::PathBuf,
    blobs: std::path::PathBuf,
    state: std::path::PathBuf,
    binding_digest: String,
}

impl Fixture {
    fn new() -> Self {
        let root = tempfile::tempdir().unwrap_or_else(|error| panic!("tempdir: {error}"));
        let workspace = root.path().join("workspace");
        let blobs = root.path().join("blobs");
        let state = root.path().join("state");
        fs::create_dir_all(workspace.join("src"))
            .unwrap_or_else(|error| panic!("workspace: {error}"));
        fs::create_dir_all(&blobs).unwrap_or_else(|error| panic!("blobs: {error}"));
        fs::write(workspace.join("src/old.txt"), b"old")
            .unwrap_or_else(|error| panic!("old: {error}"));
        fs::write(workspace.join("src/delete.txt"), b"delete")
            .unwrap_or_else(|error| panic!("delete: {error}"));
        fs::write(workspace.join("src/move.txt"), b"move")
            .unwrap_or_else(|error| panic!("move: {error}"));
        Self {
            root,
            workspace,
            blobs,
            state,
            binding_digest: common::digest("workspace-binding-1"),
        }
    }

    fn provider(&self) -> WorkspaceCommitProviderV1 {
        WorkspaceCommitProviderV1::new(WorkspaceCommitConfigV1 {
            state_directory: self.state.clone(),
            max_files: 1_000,
            max_total_bytes: 16 * 1024 * 1024,
            max_file_bytes: 4 * 1024 * 1024,
            bindings: BTreeMap::from([(
                "workspace-1".to_owned(),
                WorkspaceCommitBindingV1 {
                    workspace_root: self.workspace.clone(),
                    blob_root: self.blobs.clone(),
                    digest: self.binding_digest.clone(),
                },
            )]),
        })
    }

    fn add_blob(&self, content: &[u8], mode: u32) -> (String, String) {
        let content_digest = hex::encode(Sha256::digest(content));
        fs::write(self.blobs.join(format!("{content_digest}.blob")), content)
            .unwrap_or_else(|error| panic!("blob: {error}"));
        let descriptor = FixtureBlobDescriptor {
            content_digest: content_digest.clone(),
            length: content.len(),
            mode,
        };
        let encoded =
            serde_json::to_vec(&descriptor).unwrap_or_else(|error| panic!("descriptor: {error}"));
        let mut hash = Sha256::new();
        hash.update(b"praxis.sandbox/v2");
        hash.update([0]);
        hash.update(b"workspace-file-blob-v1");
        hash.update([0]);
        hash.update(encoded);
        (
            format!("workspace-blob-{content_digest}"),
            format!("sha256:{}", hex::encode(hash.finalize())),
        )
    }

    fn tree_revision(&self) -> String {
        let mut entries = Vec::new();
        collect_fixture_tree(&self.workspace, &self.workspace, &mut entries);
        entries.sort_by(|left, right| left.path.cmp(&right.path));
        let encoded =
            serde_json::to_vec(&entries).unwrap_or_else(|error| panic!("tree encoding: {error}"));
        format!("sha256:{}", hex::encode(Sha256::digest(encoded)))
    }

    fn payload(&self) -> WorkspaceCommitPayloadV1 {
        let expires = now_unix_nano() + 30_000_000_000;
        let (add_id, add_digest) = self.add_blob(b"added", 0o640);
        let (modify_id, modify_digest) = self.add_blob(b"new", 0o600);
        WorkspaceCommitPayloadV1 {
            workspace_binding_id: "workspace-1".to_owned(),
            workspace_digest: self.binding_digest.clone(),
            change_set: exact("changes-1", expires),
            view: exact("view-1", expires),
            base_revision: self.tree_revision(),
            file_scope_digest: common::digest("file-scope"),
            write_scopes: vec!["src".to_owned()],
            changes: vec![
                mutation("add", "src/a.txt", "", &add_id, &add_digest, 0o640),
                mutation("delete", "src/delete.txt", "", "", "", 0),
                mutation("rename", "src/move.txt", "src/moved.txt", "", "", 0),
                mutation(
                    "modify",
                    "src/old.txt",
                    "",
                    &modify_id,
                    &modify_digest,
                    0o600,
                ),
            ],
            inspection_target: None,
        }
    }
}

#[tokio::test]
async fn real_commit_applies_exact_add_modify_delete_and_rename() {
    let fixture = Fixture::new();
    let provider = fixture.provider();
    provider
        .probe()
        .await
        .unwrap_or_else(|error| panic!("probe: {error}"));
    let payload = fixture.payload();
    let prepare = request(
        EnforcementPhaseV1::Prepare,
        payload.clone(),
        "attempt-commit",
    );
    provider
        .prepare(&prepare)
        .await
        .unwrap_or_else(|error| panic!("prepare: {error}"));
    assert!(!fixture.workspace.join("src/a.txt").exists());

    let execute = request(EnforcementPhaseV1::Execute, payload, "attempt-commit");
    let result = provider
        .execute_prepared(&execute)
        .await
        .unwrap_or_else(|error| panic!("execute: {error}"));
    assert!(
        result
            .observation
            .state
            .starts_with("workspace_committed:sha256:")
    );
    assert_eq!(
        fs::read(fixture.workspace.join("src/a.txt"))
            .unwrap_or_else(|error| panic!("read added file: {error}")),
        b"added"
    );
    assert_eq!(
        fs::read(fixture.workspace.join("src/old.txt"))
            .unwrap_or_else(|error| panic!("read replaced file: {error}")),
        b"new"
    );
    assert!(!fixture.workspace.join("src/delete.txt").exists());
    assert!(!fixture.workspace.join("src/move.txt").exists());
    assert_eq!(
        fs::read(fixture.workspace.join("src/moved.txt"))
            .unwrap_or_else(|error| panic!("read moved file: {error}")),
        b"move"
    );
}

#[tokio::test]
async fn base_drift_fails_before_any_workspace_write() {
    let fixture = Fixture::new();
    let provider = fixture.provider();
    let payload = fixture.payload();
    fs::write(fixture.workspace.join("src/unplanned.txt"), b"drift")
        .unwrap_or_else(|error| panic!("drift: {error}"));
    let before = fixture.tree_revision();
    let error = common::must_error(
        provider
            .prepare(&request(
                EnforcementPhaseV1::Prepare,
                payload,
                "attempt-drift",
            ))
            .await,
    );
    assert_eq!(error.reason, ClosedReason::BindingDrift);
    assert_eq!(fixture.tree_revision(), before);
    assert!(!fixture.workspace.join("src/a.txt").exists());
}

#[tokio::test]
async fn symlink_in_workspace_fails_closed_before_commit() {
    let fixture = Fixture::new();
    let mut payload = fixture.payload();
    symlink("old.txt", fixture.workspace.join("src/link.txt"))
        .unwrap_or_else(|error| panic!("symlink: {error}"));
    let provider = fixture.provider();
    payload.base_revision = common::digest("untrusted-symlink-tree");
    let error = common::must_error(
        provider
            .prepare(&request(
                EnforcementPhaseV1::Prepare,
                payload,
                "attempt-symlink",
            ))
            .await,
    );
    assert_eq!(error.reason, ClosedReason::InvalidContract);
}

#[tokio::test]
async fn lost_execute_reply_is_recovered_only_from_exact_journal_result() {
    let fixture = Fixture::new();
    let provider = fixture.provider();
    let journal = Arc::new(
        AttemptJournal::open(fixture.root.path().join("journal"))
            .await
            .unwrap_or_else(|error| panic!("journal: {error}")),
    );
    let enforcer = DataPlaneEnforcer::new(common::reader(), journal);
    let payload = fixture.payload();
    let prepare = request(EnforcementPhaseV1::Prepare, payload.clone(), "attempt-lost");
    enforcer
        .dispatch(&provider, &prepare)
        .await
        .unwrap_or_else(|error| panic!("prepare: {error}"));
    let execute = request(EnforcementPhaseV1::Execute, payload, "attempt-lost");
    let completed = enforcer
        .dispatch(&provider, &execute)
        .await
        .unwrap_or_else(|error| panic!("execute: {error}"));
    let recovered = enforcer
        .inspect(&execute)
        .await
        .unwrap_or_else(|error| panic!("inspect lost reply: {error}"));
    assert_eq!(recovered, completed);
    assert_eq!(
        common::must_error(enforcer.dispatch(&provider, &execute).await).reason,
        ClosedReason::Conflict
    );
}

#[tokio::test(flavor = "multi_thread", worker_threads = 4)]
async fn sixty_four_concurrent_execute_calls_have_one_provider_winner() {
    let fixture = Fixture::new();
    let provider = Arc::new(fixture.provider());
    let journal = Arc::new(
        AttemptJournal::open(fixture.root.path().join("journal"))
            .await
            .unwrap_or_else(|error| panic!("journal: {error}")),
    );
    let enforcer = Arc::new(DataPlaneEnforcer::new(common::reader(), journal));
    let payload = fixture.payload();
    let prepare = request(EnforcementPhaseV1::Prepare, payload.clone(), "attempt-race");
    enforcer
        .dispatch(provider.as_ref(), &prepare)
        .await
        .unwrap_or_else(|error| panic!("prepare: {error}"));
    let execute = Arc::new(request(
        EnforcementPhaseV1::Execute,
        payload,
        "attempt-race",
    ));
    let mut tasks = Vec::new();
    for _ in 0..64 {
        let enforcer = Arc::clone(&enforcer);
        let provider = Arc::clone(&provider);
        let execute = Arc::clone(&execute);
        tasks.push(tokio::spawn(async move {
            enforcer.dispatch(provider.as_ref(), &execute).await
        }));
    }
    let mut successes = 0;
    for task in tasks {
        match task.await.unwrap_or_else(|error| panic!("join: {error}")) {
            Ok(_) => successes += 1,
            Err(error) => assert!(matches!(
                error.reason,
                ClosedReason::Conflict | ClosedReason::ProviderUnknown
            )),
        }
    }
    assert_eq!(successes, 1);
    assert_eq!(
        fs::read(fixture.workspace.join("src/a.txt"))
            .unwrap_or_else(|error| panic!("read concurrent winner output: {error}")),
        b"added"
    );
}

#[tokio::test]
async fn crash_between_workspace_swaps_is_indeterminate_then_governed_cleanup_rolls_back() {
    let fixture = Fixture::new();
    let provider = fixture.provider();
    let payload = fixture.payload();
    let prepare = request(
        EnforcementPhaseV1::Prepare,
        payload.clone(),
        "attempt-crash-window",
    );
    provider
        .prepare(&prepare)
        .await
        .unwrap_or_else(|error| panic!("prepare: {error}"));

    let key = transaction_key(&prepare.tenant_id, &prepare.attempt_id);
    let parent = fixture
        .workspace
        .parent()
        .unwrap_or_else(|| panic!("workspace must have a parent"));
    let stage = parent.join(format!(".praxis-stage-{key}"));
    let backup = parent.join(format!(".praxis-backup-{key}"));
    fs::rename(&fixture.workspace, &backup)
        .unwrap_or_else(|error| panic!("simulate first swap: {error}"));
    assert!(stage.exists() && backup.exists() && !fixture.workspace.exists());

    let inspect = recovery_request(&prepare, payload.clone(), "praxis.sandbox/inspect");
    let observed = provider
        .inspect(&inspect)
        .await
        .unwrap_or_else(|error| panic!("inspect crash window: {error}"));
    assert_eq!(observed.observation.state, "workspace_commit_indeterminate");
    assert!(!fixture.workspace.exists(), "Inspect mutated the workspace");

    let cleanup = recovery_request(&prepare, payload, "praxis.sandbox/cleanup");
    let cleaned = provider
        .cleanup(&cleanup)
        .await
        .unwrap_or_else(|error| panic!("cleanup crash window: {error}"));
    assert_eq!(cleaned.observation.state, "cleanup_absent");
    assert!(fixture.workspace.exists() && !stage.exists() && !backup.exists());
    assert_eq!(fixture.tree_revision(), prepare_workspace_base(&prepare));

    let after = provider
        .inspect(&inspect)
        .await
        .unwrap_or_else(|error| panic!("inspect after rollback: {error}"));
    assert_eq!(after.observation.state, "workspace_commit_not_applied");
}

fn request(
    phase: EnforcementPhaseV1,
    payload: WorkspaceCommitPayloadV1,
    attempt: &str,
) -> DispatchRequestV1 {
    let mut request =
        common::request_with_payload(phase, ProviderPayloadV1::WorkspaceCommit(payload));
    request.request_id = format!("workspace-{attempt}-{phase:?}");
    request.effect_kind = "praxis.sandbox/workspace-commit".to_owned();
    request.effect_id = format!("effect-{attempt}");
    request.attempt_id = attempt.to_owned();
    request.sandbox_attempt = exact(attempt, request.requested_not_after_unix_nano);
    request.runtime_enforcement.effect_id = request.effect_id.clone();
    request.runtime_enforcement.attempt_id = attempt.to_owned();
    request.runtime_enforcement.receipt_digest = common::digest(&request.request_id);
    request
        .seal()
        .unwrap_or_else(|error| panic!("workspace request: {error}"))
}

fn recovery_request(
    original: &DispatchRequestV1,
    mut payload: WorkspaceCommitPayloadV1,
    effect_kind: &str,
) -> DispatchRequestV1 {
    let target = ProviderInspectionTargetV1 {
        original_effect_kind: original.effect_kind.clone(),
        original_attempt_id: original.attempt_id.clone(),
        provider_attempt: ExactRefV1 {
            id: format!(
                "workspace-commit/{}/{}",
                original.tenant_id, original.attempt_id
            ),
            revision: 2,
            digest: common::digest("workspace-provider-attempt"),
            expires_unix_nano: original.requested_not_after_unix_nano,
        },
        original_request_digest: original.digest.clone(),
        original_payload_digest: original.payload_digest.clone(),
    };
    payload.inspection_target = None;
    let mut result = request(
        EnforcementPhaseV1::Execute,
        payload,
        &format!(
            "recovery-{}",
            effect_kind.rsplit('/').next().unwrap_or("unknown")
        ),
    );
    result.effect_kind = effect_kind.to_owned();
    let ProviderPayloadV1::WorkspaceCommit(result_payload) = &mut result.payload else {
        panic!("workspace recovery payload")
    };
    result_payload.inspection_target = Some(target);
    result
        .seal()
        .unwrap_or_else(|error| panic!("workspace recovery request: {error}"))
}

fn transaction_key(tenant: &str, attempt: &str) -> String {
    let mut hash = Sha256::new();
    hash.update(tenant.as_bytes());
    hash.update([0]);
    hash.update(attempt.as_bytes());
    hex::encode(hash.finalize())[..48].to_owned()
}

fn prepare_workspace_base(request: &DispatchRequestV1) -> String {
    let ProviderPayloadV1::WorkspaceCommit(payload) = &request.payload else {
        panic!("workspace payload")
    };
    payload.base_revision.clone()
}

fn exact(id: &str, expires: i64) -> ExactRefV1 {
    ExactRefV1 {
        id: id.to_owned(),
        revision: 1,
        digest: common::digest(id),
        expires_unix_nano: expires,
    }
}

fn mutation(
    kind: &str,
    path: &str,
    target_path: &str,
    blob_id: &str,
    blob_digest: &str,
    mode: u32,
) -> WorkspaceMutationV1 {
    WorkspaceMutationV1 {
        kind: kind.to_owned(),
        path: path.to_owned(),
        target_path: target_path.to_owned(),
        blob_id: blob_id.to_owned(),
        blob_digest: blob_digest.to_owned(),
        mode,
    }
}

fn collect_fixture_tree(
    root: &std::path::Path,
    current: &std::path::Path,
    entries: &mut Vec<FixtureTreeEntry>,
) {
    let mut paths: Vec<_> = fs::read_dir(current)
        .unwrap_or_else(|error| panic!("read tree: {error}"))
        .map(|entry| {
            entry
                .unwrap_or_else(|error| panic!("tree entry: {error}"))
                .path()
        })
        .collect();
    paths.sort();
    for path in paths {
        let metadata =
            fs::symlink_metadata(&path).unwrap_or_else(|error| panic!("tree metadata: {error}"));
        assert!(
            !metadata.file_type().is_symlink(),
            "fixture tree is not regular"
        );
        if metadata.is_dir() {
            collect_fixture_tree(root, &path, entries);
            continue;
        }
        let content = fs::read(&path).unwrap_or_else(|error| panic!("tree file: {error}"));
        let relative = path
            .strip_prefix(root)
            .unwrap_or_else(|error| panic!("tree relative: {error}"))
            .to_str()
            .unwrap_or_else(|| panic!("fixture path must be UTF-8"))
            .to_owned();
        entries.push(FixtureTreeEntry {
            path: relative,
            mode: std::os::unix::fs::PermissionsExt::mode(&metadata.permissions()) & 0o777,
            digest: hex::encode(Sha256::digest(&content)),
            length: content.len(),
        });
    }
}

#[test]
fn fixture_digest_is_sha256_contract() {
    assert_eq!(
        canonical_digest("fixture", &"workspace").unwrap_or_else(|error| panic!("digest: {error}")),
        common::digest("workspace")
    );
}

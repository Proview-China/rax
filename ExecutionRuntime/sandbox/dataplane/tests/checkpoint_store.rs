mod common;

use std::os::unix::fs::PermissionsExt;
use std::sync::Arc;
use std::sync::atomic::{AtomicUsize, Ordering};

use async_trait::async_trait;
use praxis_sandbox_dataplane::checkpoint::{
    CheckpointExactRefV1, CheckpointParticipantPhaseV1, CheckpointPreviousPhaseV1,
    CheckpointRuntimeCurrentQueryV1, CheckpointSource, CheckpointStore,
};
use praxis_sandbox_dataplane::contract::{
    DispatchRequestV1, EnforcementPhaseV1, ProviderKindV1, now_unix_nano,
};
use praxis_sandbox_dataplane::enforcer::DataPlaneEnforcer;
use praxis_sandbox_dataplane::error::{ClosedError, ClosedReason, Result};
use praxis_sandbox_dataplane::journal::AttemptJournal;
use praxis_sandbox_dataplane::provider::{Provider, ProviderResult, provider_result};
use tempfile::TempDir;

struct SourceProvider {
    source: std::path::PathBuf,
    source_calls: AtomicUsize,
}

#[async_trait]
impl Provider for SourceProvider {
    fn kind(&self) -> ProviderKindV1 {
        ProviderKindV1::WasmtimeComponent
    }

    async fn prepare(&self, request: &DispatchRequestV1) -> Result<ProviderResult> {
        provider_result(request, "ordinary_prepare")
    }

    async fn execute_prepared(&self, _request: &DispatchRequestV1) -> Result<ProviderResult> {
        Err(ClosedError::new(
            ClosedReason::Unsupported,
            "ordinary execute is not used by checkpoint test",
        ))
    }

    async fn inspect(&self, request: &DispatchRequestV1) -> Result<ProviderResult> {
        provider_result(request, "inspected")
    }

    async fn fence(&self, request: &DispatchRequestV1) -> Result<ProviderResult> {
        provider_result(request, "fenced")
    }

    async fn release(&self, request: &DispatchRequestV1) -> Result<ProviderResult> {
        provider_result(request, "released")
    }

    async fn cleanup(&self, request: &DispatchRequestV1) -> Result<ProviderResult> {
        provider_result(request, "cleaned")
    }

    async fn inspect_cleanup(&self, request: &DispatchRequestV1) -> Result<ProviderResult> {
        provider_result(request, "cleanup_absent")
    }

    async fn checkpoint_source(&self, _request: &DispatchRequestV1) -> Result<CheckpointSource> {
        self.source_calls.fetch_add(1, Ordering::SeqCst);
        Ok(CheckpointSource::Directory(self.source.clone()))
    }
}

#[tokio::test]
#[allow(clippy::too_many_lines)]
async fn checkpoint_prepare_commit_is_durable_and_lost_reply_inspects_without_provider() {
    let temp = TempDir::new().unwrap_or_else(|error| panic!("temp: {error}"));
    let source = temp.path().join("source");
    std::fs::create_dir_all(source.join("nested"))
        .unwrap_or_else(|error| panic!("source: {error}"));
    std::fs::write(source.join("nested/data"), b"checkpoint-content")
        .unwrap_or_else(|error| panic!("content: {error}"));
    std::fs::set_permissions(
        source.join("nested/data"),
        std::fs::Permissions::from_mode(0o700),
    )
    .unwrap_or_else(|error| panic!("permissions: {error}"));
    let provider = SourceProvider {
        source,
        source_calls: AtomicUsize::new(0),
    };
    let store = Arc::new(
        CheckpointStore::open(temp.path().join("checkpoint"))
            .await
            .unwrap_or_else(|error| panic!("store: {error}")),
    );
    let journal = Arc::new(
        AttemptJournal::open(temp.path().join("journal.jsonl"))
            .await
            .unwrap_or_else(|error| panic!("journal: {error}")),
    );
    let enforcer =
        DataPlaneEnforcer::new(common::reader(), journal).with_checkpoint_store(Arc::clone(&store));

    let prepare_gate = checkpoint_request(
        EnforcementPhaseV1::Prepare,
        "prepare-attempt",
        CheckpointParticipantPhaseV1::CheckpointPrepare,
        None,
    );
    enforcer
        .dispatch(&provider, &prepare_gate)
        .await
        .unwrap_or_else(|error| panic!("prepare gate: {error}"));
    let prepare_execute = checkpoint_request(
        EnforcementPhaseV1::Execute,
        "prepare-attempt",
        CheckpointParticipantPhaseV1::CheckpointPrepare,
        None,
    );
    let prepared = enforcer
        .dispatch(&provider, &prepare_execute)
        .await
        .unwrap_or_else(|error| panic!("prepare execute: {error}"));
    let prepared_artifact = checkpoint_artifact(&prepared, "prepared", "checkpoint_prepare");
    assert!(prepared_artifact.content_digest.starts_with("sha256:"));
    let staged = temp
        .path()
        .join("checkpoint/artifacts")
        .join(
            prepared_artifact
                .subject_digest
                .trim_start_matches("sha256:"),
        )
        .join("staging/nested/data");
    let staged_mode = std::fs::metadata(staged)
        .unwrap_or_else(|error| panic!("staged metadata: {error}"))
        .permissions()
        .mode();
    assert_ne!(staged_mode & 0o111, 0);
    assert_eq!(provider.source_calls.load(Ordering::SeqCst), 2);

    let recovered = store
        .inspect(&prepare_execute)
        .await
        .unwrap_or_else(|error| panic!("store inspect: {error}"));
    assert_eq!(recovered, prepared);
    assert_eq!(provider.source_calls.load(Ordering::SeqCst), 2);

    let previous = previous_prepare(&prepare_execute);
    let commit_gate = checkpoint_request(
        EnforcementPhaseV1::Prepare,
        "commit-attempt",
        CheckpointParticipantPhaseV1::CheckpointCommit,
        Some(previous.clone()),
    );
    enforcer
        .dispatch(&provider, &commit_gate)
        .await
        .unwrap_or_else(|error| panic!("commit gate: {error}"));
    let commit_execute = checkpoint_request(
        EnforcementPhaseV1::Execute,
        "commit-attempt",
        CheckpointParticipantPhaseV1::CheckpointCommit,
        Some(previous.clone()),
    );
    let committed = enforcer
        .dispatch(&provider, &commit_execute)
        .await
        .unwrap_or_else(|error| panic!("commit execute: {error}"));
    let committed_artifact = checkpoint_artifact(&committed, "committed", "checkpoint_commit");
    assert_eq!(
        committed_artifact.content_digest,
        prepared_artifact.content_digest
    );
    assert_eq!(provider.source_calls.load(Ordering::SeqCst), 2);

    let inspected = enforcer
        .inspect(&commit_execute)
        .await
        .unwrap_or_else(|error| panic!("commit inspect: {error}"));
    assert_eq!(inspected, committed);
    assert_eq!(provider.source_calls.load(Ordering::SeqCst), 2);

    let abort_after_commit = checkpoint_request(
        EnforcementPhaseV1::Prepare,
        "abort-attempt",
        CheckpointParticipantPhaseV1::CheckpointAbort,
        Some(previous),
    );
    let error = common::must_error(enforcer.dispatch(&provider, &abort_after_commit).await);
    assert_eq!(error.reason, ClosedReason::Conflict);
    assert_eq!(provider.source_calls.load(Ordering::SeqCst), 2);
}

fn checkpoint_artifact<'a>(
    result: &'a praxis_sandbox_dataplane::provider::ProviderResult,
    state: &str,
    phase: &str,
) -> &'a praxis_sandbox_dataplane::provider::CheckpointArtifactObservationV1 {
    assert_eq!(result.observation.state, format!("checkpoint_{state}"));
    let Some(artifact) = result.observation.checkpoint_artifact.as_ref() else {
        panic!("checkpoint result lacks artifact observation");
    };
    assert_eq!(artifact.state, state);
    assert_eq!(artifact.checkpoint_phase, phase);
    artifact
}

#[tokio::test]
async fn checkpoint_abort_after_commit_and_symlink_source_fail_closed() {
    let temp = TempDir::new().unwrap_or_else(|error| panic!("temp: {error}"));
    let source = temp.path().join("source");
    std::fs::create_dir_all(&source).unwrap_or_else(|error| panic!("source: {error}"));
    std::os::unix::fs::symlink("missing", source.join("escape"))
        .unwrap_or_else(|error| panic!("symlink: {error}"));
    let provider = SourceProvider {
        source,
        source_calls: AtomicUsize::new(0),
    };
    let store = CheckpointStore::open(temp.path().join("checkpoint"))
        .await
        .unwrap_or_else(|error| panic!("store: {error}"));
    let request = checkpoint_request(
        EnforcementPhaseV1::Execute,
        "prepare-attempt",
        CheckpointParticipantPhaseV1::CheckpointPrepare,
        None,
    );
    let error = common::must_error(store.execute(&provider, &request).await);
    assert_eq!(error.reason, ClosedReason::InvalidContract);
    assert!(store.inspect(&request).await.is_err());
}

fn checkpoint_request(
    enforcement: EnforcementPhaseV1,
    attempt_id: &str,
    phase: CheckpointParticipantPhaseV1,
    previous_phase: Option<CheckpointPreviousPhaseV1>,
) -> DispatchRequestV1 {
    let expires = now_unix_nano() + 60_000_000_000;
    let mut request = common::request(enforcement);
    request.request_id = format!("checkpoint-{attempt_id}-{enforcement:?}");
    request.effect_kind = "praxis.sandbox/checkpoint".to_owned();
    request.effect_id = format!("effect-{attempt_id}");
    request.attempt_id = attempt_id.to_owned();
    request.sandbox_attempt = common::exact(attempt_id, expires);
    request
        .runtime_enforcement
        .effect_id
        .clone_from(&request.effect_id);
    request.runtime_enforcement.attempt_id = attempt_id.to_owned();
    request.runtime_enforcement.phase = enforcement;
    request.runtime_enforcement.receipt_digest =
        common::digest(&format!("enforcement-{attempt_id}-{enforcement:?}"));
    request.runtime_current_query = serde_json::to_value(CheckpointRuntimeCurrentQueryV1 {
        contract_version: "praxis.sandbox/checkpoint-current-query/v1".to_owned(),
        runtime_inspect: serde_json::json!({"attempt_id": attempt_id, "enforcement": format!("{enforcement:?}")}),
        phase,
        checkpoint_attempt: checkpoint_ref("checkpoint-attempt", expires),
        barrier: checkpoint_ref("barrier", expires),
        effect_cut: checkpoint_ref("effect-cut", expires),
        reservation: checkpoint_ref(
            if phase == CheckpointParticipantPhaseV1::CheckpointPrepare {
                "prepare-reservation"
            } else {
                "successor-reservation"
            },
            expires,
        ),
        participant: checkpoint_ref("participant", expires),
        previous_phase,
        projection_digest: common::digest("checkpoint-projection"),
        expires_unix_nano: expires,
    })
    .unwrap_or_else(|error| panic!("query: {error}"));
    request.requested_not_after_unix_nano = expires;
    request.execution_binding.expires_unix_nano = expires;
    request.runtime_enforcement.expires_unix_nano = expires;
    request.digest.clear();
    request.runtime_current_query_digest.clear();
    request.payload_digest.clear();
    request
        .seal()
        .unwrap_or_else(|error| panic!("checkpoint request: {error}"))
}

fn checkpoint_ref(id: &str, expires: i64) -> CheckpointExactRefV1 {
    CheckpointExactRefV1 {
        id: id.to_owned(),
        revision: 1,
        digest: common::digest(id),
        expires_unix_nano: expires,
    }
}

fn previous_prepare(request: &DispatchRequestV1) -> CheckpointPreviousPhaseV1 {
    let query = CheckpointRuntimeCurrentQueryV1::from_request(request)
        .unwrap_or_else(|error| panic!("prepare query: {error}"));
    CheckpointPreviousPhaseV1 {
        reservation: query.reservation,
        closure_id: "prepare-closure".to_owned(),
        closure_digest: common::digest("prepare-closure"),
        state: "prepared".to_owned(),
        expires_unix_nano: query.expires_unix_nano,
    }
}

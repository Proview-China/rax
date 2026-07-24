use std::collections::BTreeMap;
use std::fs::{self, File, OpenOptions};
use std::io::{Read as _, Write as _};
use std::os::unix::fs::PermissionsExt as _;
use std::path::{Component, Path, PathBuf};

use async_trait::async_trait;
use serde::{Deserialize, Serialize};
use sha2::{Digest as _, Sha256};

use crate::checkpoint::CheckpointSource;
use crate::contract::{
    DispatchRequestV1, ProviderKindV1, ProviderPayloadV1, WorkspaceCommitPayloadV1,
    WorkspaceMutationV1, canonical_digest,
};
use crate::error::{ClosedError, ClosedReason, Result};
use crate::provider::{Provider, ProviderResult, WorkspaceCommitObservationV1, provider_result};

#[derive(Clone, Debug, Deserialize)]
#[serde(deny_unknown_fields)]
pub struct WorkspaceCommitBindingV1 {
    pub workspace_root: PathBuf,
    pub blob_root: PathBuf,
    pub digest: String,
}

#[derive(Clone, Debug, Deserialize)]
#[serde(deny_unknown_fields)]
pub struct WorkspaceCommitConfigV1 {
    pub state_directory: PathBuf,
    pub max_files: usize,
    pub max_total_bytes: u64,
    pub max_file_bytes: u64,
    pub bindings: BTreeMap<String, WorkspaceCommitBindingV1>,
}

#[derive(Clone, Debug, Serialize, Deserialize)]
#[serde(deny_unknown_fields)]
struct CommitRecordV1 {
    attempt_id: String,
    payload_digest: String,
    change_set_id: String,
    change_set_revision: u64,
    change_set_digest: String,
    base_revision: String,
    committed_revision: String,
    state: String,
}

#[derive(Clone, Debug, Serialize)]
struct TreeEntryV1 {
    path: String,
    mode: u32,
    digest: String,
    length: usize,
}

#[derive(Clone, Debug, Serialize)]
struct BlobDescriptorV1 {
    content_digest: String,
    length: usize,
    mode: u32,
}

pub struct WorkspaceCommitProviderV1 {
    config: WorkspaceCommitConfigV1,
}

impl WorkspaceCommitProviderV1 {
    #[must_use]
    pub fn new(config: WorkspaceCommitConfigV1) -> Self {
        Self { config }
    }

    pub async fn probe(&self) -> Result<()> {
        let config = self.config.clone();
        tokio::task::spawn_blocking(move || validate_config(&config))
            .await
            .map_err(ClosedError::internal)?
    }

    async fn prepare_exact(&self, request: &DispatchRequestV1) -> Result<CommitRecordV1> {
        let config = self.config.clone();
        let request = request.clone();
        tokio::task::spawn_blocking(move || prepare_sync(&config, &request))
            .await
            .map_err(ClosedError::internal)?
    }

    async fn execute_exact(&self, request: &DispatchRequestV1) -> Result<CommitRecordV1> {
        let config = self.config.clone();
        let request = request.clone();
        tokio::task::spawn_blocking(move || execute_sync(&config, &request))
            .await
            .map_err(ClosedError::internal)?
    }

    async fn inspect_exact(&self, request: &DispatchRequestV1) -> Result<CommitRecordV1> {
        let config = self.config.clone();
        let request = request.clone();
        tokio::task::spawn_blocking(move || inspect_sync(&config, &request))
            .await
            .map_err(ClosedError::internal)?
    }

    async fn cleanup_exact(&self, request: &DispatchRequestV1) -> Result<String> {
        let config = self.config.clone();
        let request = request.clone();
        tokio::task::spawn_blocking(move || cleanup_sync(&config, &request))
            .await
            .map_err(ClosedError::internal)?
    }

    async fn inspect_cleanup_exact(&self, request: &DispatchRequestV1) -> Result<String> {
        let config = self.config.clone();
        let request = request.clone();
        tokio::task::spawn_blocking(move || inspect_cleanup_sync(&config, &request))
            .await
            .map_err(ClosedError::internal)?
    }
}

#[async_trait]
impl Provider for WorkspaceCommitProviderV1 {
    fn kind(&self) -> ProviderKindV1 {
        ProviderKindV1::WorkspaceCommit
    }

    async fn prepare(&self, request: &DispatchRequestV1) -> Result<ProviderResult> {
        if request.effect_kind == "praxis.sandbox/inspect" {
            return provider_result(request, "workspace_inspection_prepared");
        }
        if request.effect_kind != "praxis.sandbox/workspace-commit" {
            return Err(ClosedError::new(
                ClosedReason::Unsupported,
                "workspace commit Provider received another effect",
            ));
        }
        let record = self.prepare_exact(request).await?;
        provider_result(request, "workspace_commit_prepared")?
            .with_workspace_commit(workspace_observation(request, &record)?)
    }

    async fn execute_prepared(&self, request: &DispatchRequestV1) -> Result<ProviderResult> {
        let record = self.execute_exact(request).await?;
        provider_result(
            request,
            &format!("workspace_committed:{}", record.committed_revision),
        )?
        .with_workspace_commit(workspace_observation(request, &record)?)
    }

    async fn inspect(&self, request: &DispatchRequestV1) -> Result<ProviderResult> {
        let record = self.inspect_exact(request).await?;
        let state = match record.state.as_str() {
            "committed" => format!("workspace_committed:{}", record.committed_revision),
            "not_applied" => "workspace_commit_not_applied".to_owned(),
            _ => "workspace_commit_indeterminate".to_owned(),
        };
        provider_result(request, &state)?
            .with_workspace_commit(workspace_observation(request, &record)?)
    }

    async fn fence(&self, _request: &DispatchRequestV1) -> Result<ProviderResult> {
        unsupported("workspace commit does not own execution fencing")
    }

    async fn release(&self, _request: &DispatchRequestV1) -> Result<ProviderResult> {
        unsupported("workspace commit does not own environment release")
    }

    async fn cleanup(&self, request: &DispatchRequestV1) -> Result<ProviderResult> {
        provider_result(request, &self.cleanup_exact(request).await?)
    }

    async fn inspect_cleanup(&self, request: &DispatchRequestV1) -> Result<ProviderResult> {
        provider_result(request, &self.inspect_cleanup_exact(request).await?)
    }

    async fn checkpoint_source(&self, _request: &DispatchRequestV1) -> Result<CheckpointSource> {
        Err(ClosedError::new(
            ClosedReason::Unsupported,
            "workspace commit is not a checkpoint source",
        ))
    }
}

fn unsupported<T>(message: &str) -> Result<T> {
    Err(ClosedError::new(ClosedReason::Unsupported, message))
}

fn prepare_sync(
    config: &WorkspaceCommitConfigV1,
    request: &DispatchRequestV1,
) -> Result<CommitRecordV1> {
    validate_config(config)?;
    let payload = workspace_payload(request)?;
    let binding = exact_binding(config, payload)?;
    let record_path = record_path(config, &request.tenant_id, &request.attempt_id);
    if let Some(existing) = read_json_optional::<CommitRecordV1>(&record_path)? {
        validate_record(&existing, request, payload)?;
        return Ok(existing);
    }
    let current = tree_revision(binding, &binding.workspace_root, config)?;
    if current != payload.base_revision {
        return Err(ClosedError::new(
            ClosedReason::BindingDrift,
            "workspace base revision drifted before prepare",
        ));
    }
    let (stage, backup) = transaction_paths(binding, &request.tenant_id, &request.attempt_id)?;
    remove_if_exists(&stage)?;
    if backup.exists() {
        return Err(ClosedError::new(
            ClosedReason::Conflict,
            "workspace commit has an unresolved backup residual",
        ));
    }
    copy_tree(&binding.workspace_root, &stage, config)?;
    if let Err(error) = apply_changes(&stage, &binding.blob_root, payload) {
        let _ = remove_if_exists(&stage);
        return Err(error);
    }
    let committed_revision = tree_revision(binding, &stage, config)?;
    if committed_revision == payload.base_revision {
        let _ = remove_if_exists(&stage);
        return Err(ClosedError::new(
            ClosedReason::Conflict,
            "workspace commit mutations do not change the exact tree",
        ));
    }
    let record = CommitRecordV1 {
        attempt_id: request.attempt_id.clone(),
        payload_digest: request.payload_digest.clone(),
        change_set_id: payload.change_set.id.clone(),
        change_set_revision: payload.change_set.revision,
        change_set_digest: payload.change_set.digest.clone(),
        base_revision: payload.base_revision.clone(),
        committed_revision,
        state: "prepared".to_owned(),
    };
    write_json_atomic(&record_path, &record)?;
    Ok(record)
}

fn execute_sync(
    config: &WorkspaceCommitConfigV1,
    request: &DispatchRequestV1,
) -> Result<CommitRecordV1> {
    let payload = workspace_payload(request)?;
    let binding = exact_binding(config, payload)?;
    let record_path = record_path(config, &request.tenant_id, &request.attempt_id);
    let mut record = read_json_optional::<CommitRecordV1>(&record_path)?.ok_or_else(|| {
        ClosedError::new(
            ClosedReason::NotFoundObservation,
            "workspace commit execute has no durable prepare",
        )
    })?;
    validate_record(&record, request, payload)?;
    let current = tree_revision(binding, &binding.workspace_root, config)?;
    if record.state == "committed" {
        if current == record.committed_revision {
            return Ok(record);
        }
        return Err(ClosedError::new(
            ClosedReason::BindingDrift,
            "committed workspace tree drifted after exact commit",
        ));
    }
    if current != record.base_revision {
        return Err(ClosedError::new(
            ClosedReason::BindingDrift,
            "workspace base revision drifted between prepare and execute",
        ));
    }
    let (stage, backup) = transaction_paths(binding, &request.tenant_id, &request.attempt_id)?;
    if tree_revision(binding, &stage, config)? != record.committed_revision || backup.exists() {
        return Err(ClosedError::new(
            ClosedReason::Conflict,
            "workspace commit stage or backup is not in its prepared state",
        ));
    }
    fs::rename(&binding.workspace_root, &backup).map_err(ClosedError::internal)?;
    if let Err(error) = fs::rename(&stage, &binding.workspace_root) {
        let rollback = fs::rename(&backup, &binding.workspace_root);
        return if rollback.is_ok() {
            Err(ClosedError::internal(error))
        } else {
            Err(ClosedError::new(
                ClosedReason::ProviderUnknown,
                "workspace swap and rollback both failed",
            ))
        };
    }
    record.state = "committed".to_owned();
    write_json_atomic(&record_path, &record)?;
    remove_if_exists(&backup)?;
    Ok(record)
}

fn inspect_sync(
    config: &WorkspaceCommitConfigV1,
    request: &DispatchRequestV1,
) -> Result<CommitRecordV1> {
    let payload = workspace_payload(request)?;
    let target = payload.inspection_target.as_ref().ok_or_else(|| {
        ClosedError::new(
            ClosedReason::InvalidContract,
            "workspace commit Inspect lacks its exact original target",
        )
    })?;
    let binding = recovery_binding(config, payload)?;
    let record_path = record_path(config, &request.tenant_id, &target.original_attempt_id);
    let record = read_json_optional::<CommitRecordV1>(&record_path)?.ok_or_else(|| {
        ClosedError::new(
            ClosedReason::NotFoundObservation,
            "workspace commit attempt was not found",
        )
    })?;
    if record.payload_digest != target.original_payload_digest {
        return Err(ClosedError::new(
            ClosedReason::Conflict,
            "workspace commit Inspect payload provenance drifted",
        ));
    }
    let current = tree_revision_optional(binding, &binding.workspace_root, config)?;
    if current.as_deref() == Some(record.committed_revision.as_str()) {
        let mut result = record;
        result.state = "committed".to_owned();
        return Ok(result);
    }
    if current.as_deref() == Some(record.base_revision.as_str()) {
        let mut result = record;
        result.state = "not_applied".to_owned();
        return Ok(result);
    }
    let mut result = record;
    result.state = "indeterminate".to_owned();
    Ok(result)
}

fn cleanup_sync(config: &WorkspaceCommitConfigV1, request: &DispatchRequestV1) -> Result<String> {
    let payload = workspace_payload(request)?;
    let target = payload.inspection_target.as_ref().ok_or_else(|| {
        ClosedError::new(
            ClosedReason::InvalidContract,
            "workspace cleanup lacks its exact original commit target",
        )
    })?;
    let binding = recovery_binding(config, payload)?;
    let record = read_json_optional::<CommitRecordV1>(&record_path(
        config,
        &request.tenant_id,
        &target.original_attempt_id,
    ))?
    .ok_or_else(|| {
        ClosedError::new(
            ClosedReason::NotFoundObservation,
            "workspace cleanup target was not found",
        )
    })?;
    validate_recovery_record(&record, payload, target)?;
    let (stage, backup) =
        transaction_paths(binding, &request.tenant_id, &target.original_attempt_id)?;
    let current = tree_revision_optional(binding, &binding.workspace_root, config)?;
    let staged = tree_revision_optional(binding, &stage, config)?;
    let backed_up = tree_revision_optional(binding, &backup, config)?;

    match current.as_deref() {
        Some(value) if value == record.committed_revision => {
            if staged.is_some_and(|value| value != record.committed_revision)
                || backed_up.is_some_and(|value| value != record.base_revision)
            {
                return Ok("residual_present".to_owned());
            }
            remove_if_exists(&stage)?;
            remove_if_exists(&backup)?;
        }
        Some(value) if value == record.base_revision => {
            if staged.is_some_and(|value| value != record.committed_revision)
                || backed_up.is_some_and(|value| value != record.base_revision)
            {
                return Ok("residual_present".to_owned());
            }
            remove_if_exists(&stage)?;
            remove_if_exists(&backup)?;
        }
        None if backed_up.as_deref() == Some(record.base_revision.as_str())
            && (staged.is_none()
                || staged.as_deref() == Some(record.committed_revision.as_str())) =>
        {
            fs::rename(&backup, &binding.workspace_root).map_err(ClosedError::internal)?;
            remove_if_exists(&stage)?;
        }
        _ => return Ok("residual_present".to_owned()),
    }
    inspect_cleanup_sync(config, request)
}

fn inspect_cleanup_sync(
    config: &WorkspaceCommitConfigV1,
    request: &DispatchRequestV1,
) -> Result<String> {
    let payload = workspace_payload(request)?;
    let target = payload.inspection_target.as_ref().ok_or_else(|| {
        ClosedError::new(
            ClosedReason::InvalidContract,
            "workspace cleanup Inspect lacks its exact original target",
        )
    })?;
    let binding = recovery_binding(config, payload)?;
    let record = read_json_optional::<CommitRecordV1>(&record_path(
        config,
        &request.tenant_id,
        &target.original_attempt_id,
    ))?
    .ok_or_else(|| {
        ClosedError::new(
            ClosedReason::NotFoundObservation,
            "workspace cleanup target was not found",
        )
    })?;
    validate_recovery_record(&record, payload, target)?;
    let (stage, backup) =
        transaction_paths(binding, &request.tenant_id, &target.original_attempt_id)?;
    let current = tree_revision_optional(binding, &binding.workspace_root, config)?;
    let clean = matches!(current.as_deref(), Some(value) if value == record.base_revision || value == record.committed_revision)
        && !stage.exists()
        && !backup.exists();
    Ok(if clean {
        "cleanup_absent"
    } else {
        "residual_present"
    }
    .to_owned())
}

fn workspace_observation(
    request: &DispatchRequestV1,
    record: &CommitRecordV1,
) -> Result<WorkspaceCommitObservationV1> {
    let payload = workspace_payload(request)?;
    let now = crate::contract::now_unix_nano();
    Ok(WorkspaceCommitObservationV1 {
        contract_version: "praxis.sandbox/workspace-commit-observation/v1".to_owned(),
        change_set: payload.change_set.clone(),
        view: payload.view.clone(),
        base_revision: record.base_revision.clone(),
        committed_revision: record.committed_revision.clone(),
        state: record.state.clone(),
        recorded_unix_nano: now,
        expires_unix_nano: request
            .requested_not_after_unix_nano
            .min(request.sandbox_attempt.expires_unix_nano)
            .min(request.execution_binding.expires_unix_nano)
            .min(request.runtime_enforcement.expires_unix_nano)
            .min(payload.change_set.expires_unix_nano)
            .min(payload.view.expires_unix_nano),
    })
}

fn validate_config(config: &WorkspaceCommitConfigV1) -> Result<()> {
    if !config.state_directory.is_absolute()
        || config.max_files == 0
        || config.max_total_bytes == 0
        || config.max_file_bytes == 0
        || config.max_file_bytes > config.max_total_bytes
    {
        return Err(ClosedError::new(
            ClosedReason::InvalidContract,
            "workspace commit configuration is invalid",
        ));
    }
    fs::create_dir_all(&config.state_directory).map_err(ClosedError::internal)?;
    ensure_real_directory(&config.state_directory)?;
    for binding in config.bindings.values() {
        if !binding.workspace_root.is_absolute()
            || !binding.blob_root.is_absolute()
            || !crate::contract::valid_digest(&binding.digest)
        {
            return Err(ClosedError::new(
                ClosedReason::InvalidContract,
                "workspace commit binding is incomplete",
            ));
        }
        ensure_real_directory(&binding.workspace_root)?;
        ensure_real_directory(&binding.blob_root)?;
        let workspace_root =
            fs::canonicalize(&binding.workspace_root).map_err(ClosedError::internal)?;
        let blob_root = fs::canonicalize(&binding.blob_root).map_err(ClosedError::internal)?;
        if workspace_root.starts_with(&blob_root) || blob_root.starts_with(&workspace_root) {
            return Err(ClosedError::new(
                ClosedReason::InvalidContract,
                "workspace and blob roots overlap",
            ));
        }
    }
    Ok(())
}

fn workspace_payload(request: &DispatchRequestV1) -> Result<&WorkspaceCommitPayloadV1> {
    let ProviderPayloadV1::WorkspaceCommit(payload) = &request.payload else {
        return Err(ClosedError::new(
            ClosedReason::BindingDrift,
            "non-workspace payload reached workspace commit Provider",
        ));
    };
    Ok(payload)
}

fn exact_binding<'a>(
    config: &'a WorkspaceCommitConfigV1,
    payload: &WorkspaceCommitPayloadV1,
) -> Result<&'a WorkspaceCommitBindingV1> {
    let binding = config
        .bindings
        .get(&payload.workspace_binding_id)
        .ok_or_else(|| {
            ClosedError::new(
                ClosedReason::NotFoundObservation,
                "workspace commit binding was not found",
            )
        })?;
    if binding.digest != payload.workspace_digest {
        return Err(ClosedError::new(
            ClosedReason::InvalidDigest,
            "workspace commit binding digest drifted",
        ));
    }
    ensure_real_directory(&binding.workspace_root)?;
    ensure_real_directory(&binding.blob_root)?;
    Ok(binding)
}

fn recovery_binding<'a>(
    config: &'a WorkspaceCommitConfigV1,
    payload: &WorkspaceCommitPayloadV1,
) -> Result<&'a WorkspaceCommitBindingV1> {
    let binding = config
        .bindings
        .get(&payload.workspace_binding_id)
        .ok_or_else(|| {
            ClosedError::new(
                ClosedReason::NotFoundObservation,
                "workspace commit binding was not found",
            )
        })?;
    if binding.digest != payload.workspace_digest || !binding.workspace_root.is_absolute() {
        return Err(ClosedError::new(
            ClosedReason::InvalidDigest,
            "workspace commit recovery binding drifted",
        ));
    }
    ensure_real_directory(&binding.blob_root)?;
    ensure_real_directory(binding.workspace_root.parent().ok_or_else(|| {
        ClosedError::new(
            ClosedReason::InvalidContract,
            "workspace recovery binding has no parent",
        )
    })?)?;
    if binding.workspace_root.exists() {
        ensure_real_directory(&binding.workspace_root)?;
    }
    Ok(binding)
}

fn validate_record(
    record: &CommitRecordV1,
    request: &DispatchRequestV1,
    payload: &WorkspaceCommitPayloadV1,
) -> Result<()> {
    if record.attempt_id != request.attempt_id
        || record.payload_digest != request.payload_digest
        || record.change_set_id != payload.change_set.id
        || record.change_set_revision != payload.change_set.revision
        || record.change_set_digest != payload.change_set.digest
        || record.base_revision != payload.base_revision
        || !matches!(record.state.as_str(), "prepared" | "committed")
    {
        return Err(ClosedError::new(
            ClosedReason::Conflict,
            "workspace commit attempt already binds different immutable content",
        ));
    }
    Ok(())
}

fn validate_recovery_record(
    record: &CommitRecordV1,
    payload: &WorkspaceCommitPayloadV1,
    target: &crate::contract::ProviderInspectionTargetV1,
) -> Result<()> {
    if record.attempt_id != target.original_attempt_id
        || record.payload_digest != target.original_payload_digest
        || record.change_set_id != payload.change_set.id
        || record.change_set_revision != payload.change_set.revision
        || record.change_set_digest != payload.change_set.digest
        || record.base_revision != payload.base_revision
        || !matches!(record.state.as_str(), "prepared" | "committed")
    {
        return Err(ClosedError::new(
            ClosedReason::Conflict,
            "workspace recovery target binds different immutable content",
        ));
    }
    Ok(())
}

fn apply_changes(root: &Path, blob_root: &Path, payload: &WorkspaceCommitPayloadV1) -> Result<()> {
    for change in &payload.changes {
        let source = safe_child(root, &change.path, true)?;
        match change.kind.as_str() {
            "add" | "modify" => apply_blob(blob_root, &source, change)?,
            "delete" => {
                let metadata = fs::symlink_metadata(&source).map_err(|error| {
                    if error.kind() == std::io::ErrorKind::NotFound {
                        ClosedError::new(
                            ClosedReason::Conflict,
                            "workspace delete source is absent",
                        )
                    } else {
                        ClosedError::internal(error)
                    }
                })?;
                if !metadata.is_file() || metadata.file_type().is_symlink() {
                    return Err(ClosedError::new(
                        ClosedReason::Conflict,
                        "workspace delete source is not a regular file",
                    ));
                }
                fs::remove_file(source).map_err(ClosedError::internal)?;
            }
            "rename" => {
                let target = safe_child(root, &change.target_path, true)?;
                if target.exists() {
                    return Err(ClosedError::new(
                        ClosedReason::Conflict,
                        "workspace rename target already exists",
                    ));
                }
                if let Some(parent) = target.parent() {
                    fs::create_dir_all(parent).map_err(ClosedError::internal)?;
                    ensure_no_symlink_prefix(root, parent)?;
                }
                fs::rename(source, target).map_err(ClosedError::internal)?;
            }
            _ => return unsupported("workspace mutation kind is unsupported"),
        }
    }
    Ok(())
}

fn apply_blob(blob_root: &Path, target: &Path, change: &WorkspaceMutationV1) -> Result<()> {
    let content_digest = change
        .blob_id
        .strip_prefix("workspace-blob-")
        .ok_or_else(|| {
            ClosedError::new(
                ClosedReason::InvalidContract,
                "workspace blob identity is invalid",
            )
        })?;
    let blob = blob_root.join(format!("{content_digest}.blob"));
    let metadata = fs::symlink_metadata(&blob).map_err(|_| {
        ClosedError::new(
            ClosedReason::NotFoundObservation,
            "workspace blob was not found",
        )
    })?;
    if !metadata.is_file() || metadata.file_type().is_symlink() {
        return Err(ClosedError::new(
            ClosedReason::InvalidContract,
            "workspace blob is not a regular file",
        ));
    }
    let mut content = Vec::new();
    File::open(&blob)
        .and_then(|mut file| file.read_to_end(&mut content))
        .map_err(ClosedError::internal)?;
    let actual = hex::encode(Sha256::digest(&content));
    if actual != content_digest {
        return Err(ClosedError::new(
            ClosedReason::InvalidDigest,
            "workspace blob content digest drifted",
        ));
    }
    let descriptor = BlobDescriptorV1 {
        content_digest: actual.clone(),
        length: content.len(),
        mode: change.mode,
    };
    let descriptor_digest = sandbox_digest("workspace-file-blob-v1", &descriptor)?;
    if change.blob_digest != format!("sha256:{descriptor_digest}") {
        return Err(ClosedError::new(
            ClosedReason::InvalidDigest,
            "workspace blob descriptor digest drifted",
        ));
    }
    let parent = target.parent().ok_or_else(|| {
        ClosedError::new(
            ClosedReason::InvalidArgument,
            "workspace target has no parent",
        )
    })?;
    fs::create_dir_all(parent).map_err(ClosedError::internal)?;
    let temporary = parent.join(format!(".praxis-commit-{}.next", &actual[..24]));
    let mut file = OpenOptions::new()
        .create_new(true)
        .write(true)
        .open(&temporary)
        .map_err(ClosedError::internal)?;
    file.set_permissions(fs::Permissions::from_mode(change.mode))
        .map_err(ClosedError::internal)?;
    if let Err(error) = file.write_all(&content).and_then(|()| file.sync_all()) {
        let _ = fs::remove_file(&temporary);
        return Err(ClosedError::internal(error));
    }
    if let Err(error) = fs::rename(&temporary, target) {
        let _ = fs::remove_file(&temporary);
        return Err(ClosedError::internal(error));
    }
    Ok(())
}

fn tree_revision(
    binding: &WorkspaceCommitBindingV1,
    root: &Path,
    config: &WorkspaceCommitConfigV1,
) -> Result<String> {
    let canonical_root = fs::canonicalize(root).map_err(ClosedError::internal)?;
    let canonical_workspace_parent =
        fs::canonicalize(binding.workspace_root.parent().ok_or_else(|| {
            ClosedError::new(
                ClosedReason::InvalidContract,
                "workspace root has no parent",
            )
        })?)
        .map_err(ClosedError::internal)?;
    if !canonical_root.starts_with(canonical_workspace_parent) {
        return Err(ClosedError::new(
            ClosedReason::BindingDrift,
            "workspace transaction root escaped its binding parent",
        ));
    }
    let mut entries = Vec::new();
    let mut total = 0_u64;
    collect_tree(
        &canonical_root,
        &canonical_root,
        &mut entries,
        &mut total,
        config,
    )?;
    entries.sort_by(|left, right| left.path.cmp(&right.path));
    let encoded = serde_json::to_vec(&entries).map_err(ClosedError::internal)?;
    Ok(format!("sha256:{}", hex::encode(Sha256::digest(encoded))))
}

fn tree_revision_optional(
    binding: &WorkspaceCommitBindingV1,
    root: &Path,
    config: &WorkspaceCommitConfigV1,
) -> Result<Option<String>> {
    match fs::symlink_metadata(root) {
        Ok(metadata) if metadata.is_dir() && !metadata.file_type().is_symlink() => {
            tree_revision(binding, root, config).map(Some)
        }
        Ok(_) => Err(ClosedError::new(
            ClosedReason::BindingDrift,
            "workspace transaction path is not a real directory",
        )),
        Err(error) if error.kind() == std::io::ErrorKind::NotFound => Ok(None),
        Err(error) => Err(ClosedError::internal(error)),
    }
}

fn collect_tree(
    root: &Path,
    current: &Path,
    entries: &mut Vec<TreeEntryV1>,
    total: &mut u64,
    config: &WorkspaceCommitConfigV1,
) -> Result<()> {
    for entry in fs::read_dir(current).map_err(ClosedError::internal)? {
        let entry = entry.map_err(ClosedError::internal)?;
        let path = entry.path();
        let metadata = fs::symlink_metadata(&path).map_err(ClosedError::internal)?;
        if metadata.file_type().is_symlink() || (!metadata.is_dir() && !metadata.is_file()) {
            return Err(ClosedError::new(
                ClosedReason::InvalidContract,
                "workspace tree contains a symlink or special file",
            ));
        }
        if metadata.is_dir() {
            collect_tree(root, &path, entries, total, config)?;
            continue;
        }
        let length = metadata.len();
        *total = total.saturating_add(length);
        if entries.len() >= config.max_files
            || length > config.max_file_bytes
            || *total > config.max_total_bytes
        {
            return Err(ClosedError::new(
                ClosedReason::ResourceLimit,
                "workspace commit tree exceeds configured limits",
            ));
        }
        let mut content = Vec::new();
        File::open(&path)
            .and_then(|mut file| file.read_to_end(&mut content))
            .map_err(ClosedError::internal)?;
        let relative = path.strip_prefix(root).map_err(ClosedError::internal)?;
        entries.push(TreeEntryV1 {
            path: relative.to_string_lossy().replace('\\', "/"),
            mode: metadata.permissions().mode() & 0o777,
            digest: hex::encode(Sha256::digest(&content)),
            length: content.len(),
        });
    }
    Ok(())
}

fn copy_tree(source: &Path, target: &Path, config: &WorkspaceCommitConfigV1) -> Result<()> {
    fs::create_dir(target).map_err(ClosedError::internal)?;
    let mut count = 0_usize;
    let mut total = 0_u64;
    copy_tree_inner(source, target, &mut count, &mut total, config)
}

fn copy_tree_inner(
    source: &Path,
    target: &Path,
    count: &mut usize,
    total: &mut u64,
    config: &WorkspaceCommitConfigV1,
) -> Result<()> {
    for entry in fs::read_dir(source).map_err(ClosedError::internal)? {
        let entry = entry.map_err(ClosedError::internal)?;
        let source_path = entry.path();
        let target_path = target.join(entry.file_name());
        let metadata = fs::symlink_metadata(&source_path).map_err(ClosedError::internal)?;
        if metadata.file_type().is_symlink() || (!metadata.is_dir() && !metadata.is_file()) {
            return Err(ClosedError::new(
                ClosedReason::InvalidContract,
                "workspace copy encountered a symlink or special file",
            ));
        }
        if metadata.is_dir() {
            fs::create_dir(&target_path).map_err(ClosedError::internal)?;
            fs::set_permissions(
                &target_path,
                fs::Permissions::from_mode(metadata.permissions().mode() & 0o777),
            )
            .map_err(ClosedError::internal)?;
            copy_tree_inner(&source_path, &target_path, count, total, config)?;
        } else {
            *count += 1;
            *total = total.saturating_add(metadata.len());
            if *count > config.max_files
                || metadata.len() > config.max_file_bytes
                || *total > config.max_total_bytes
            {
                return Err(ClosedError::new(
                    ClosedReason::ResourceLimit,
                    "workspace copy exceeds configured limits",
                ));
            }
            fs::copy(&source_path, &target_path).map_err(ClosedError::internal)?;
            fs::set_permissions(
                &target_path,
                fs::Permissions::from_mode(metadata.permissions().mode() & 0o777),
            )
            .map_err(ClosedError::internal)?;
        }
    }
    Ok(())
}

fn safe_child(root: &Path, logical: &str, allow_missing_leaf: bool) -> Result<PathBuf> {
    let mut current = root.to_path_buf();
    let components: Vec<_> = Path::new(logical).components().collect();
    for (index, component) in components.iter().enumerate() {
        let Component::Normal(part) = component else {
            return Err(ClosedError::new(
                ClosedReason::InvalidArgument,
                "workspace path is not canonical",
            ));
        };
        current.push(part);
        match fs::symlink_metadata(&current) {
            Ok(metadata) if metadata.file_type().is_symlink() => {
                return Err(ClosedError::new(
                    ClosedReason::BindingDrift,
                    "workspace path traverses a symlink",
                ));
            }
            Ok(metadata) if index + 1 < components.len() && !metadata.is_dir() => {
                return Err(ClosedError::new(
                    ClosedReason::Conflict,
                    "workspace path parent is not a directory",
                ));
            }
            Ok(_) => {}
            Err(error)
                if error.kind() == std::io::ErrorKind::NotFound
                    && (allow_missing_leaf || index + 1 < components.len()) => {}
            Err(error) => return Err(ClosedError::internal(error)),
        }
    }
    Ok(current)
}

fn ensure_no_symlink_prefix(root: &Path, path: &Path) -> Result<()> {
    let relative = path.strip_prefix(root).map_err(|_| {
        ClosedError::new(ClosedReason::BindingDrift, "workspace parent escaped root")
    })?;
    let _ = safe_child(root, &relative.to_string_lossy(), true)?;
    Ok(())
}

fn transaction_paths(
    binding: &WorkspaceCommitBindingV1,
    tenant: &str,
    attempt: &str,
) -> Result<(PathBuf, PathBuf)> {
    let parent = binding.workspace_root.parent().ok_or_else(|| {
        ClosedError::new(
            ClosedReason::InvalidContract,
            "workspace root has no parent",
        )
    })?;
    let key = short_key(tenant, attempt);
    Ok((
        parent.join(format!(".praxis-stage-{key}")),
        parent.join(format!(".praxis-backup-{key}")),
    ))
}

fn record_path(config: &WorkspaceCommitConfigV1, tenant: &str, attempt: &str) -> PathBuf {
    config
        .state_directory
        .join(format!("{}.json", short_key(tenant, attempt)))
}

fn short_key(tenant: &str, attempt: &str) -> String {
    let mut hash = Sha256::new();
    hash.update(tenant.as_bytes());
    hash.update([0]);
    hash.update(attempt.as_bytes());
    hex::encode(hash.finalize())[..48].to_owned()
}

fn ensure_real_directory(path: &Path) -> Result<()> {
    let metadata = fs::symlink_metadata(path).map_err(|_| {
        ClosedError::new(
            ClosedReason::NotFoundObservation,
            "configured workspace directory is unavailable",
        )
    })?;
    if !metadata.is_dir() || metadata.file_type().is_symlink() {
        return Err(ClosedError::new(
            ClosedReason::InvalidContract,
            "configured workspace directory is not real",
        ));
    }
    Ok(())
}

fn remove_if_exists(path: &Path) -> Result<()> {
    match fs::remove_dir_all(path) {
        Ok(()) => Ok(()),
        Err(error) if error.kind() == std::io::ErrorKind::NotFound => Ok(()),
        Err(error) => Err(ClosedError::internal(error)),
    }
}

fn read_json_optional<T: for<'de> Deserialize<'de>>(path: &Path) -> Result<Option<T>> {
    match fs::read(path) {
        Ok(bytes) => serde_json::from_slice(&bytes).map(Some).map_err(|_| {
            ClosedError::new(
                ClosedReason::InvalidContract,
                "workspace commit state is corrupt",
            )
        }),
        Err(error) if error.kind() == std::io::ErrorKind::NotFound => Ok(None),
        Err(error) => Err(ClosedError::internal(error)),
    }
}

fn write_json_atomic<T: Serialize>(path: &Path, value: &T) -> Result<()> {
    let parent = path.parent().ok_or_else(|| {
        ClosedError::new(ClosedReason::InvalidContract, "state path has no parent")
    })?;
    fs::create_dir_all(parent).map_err(ClosedError::internal)?;
    let temporary = path.with_extension(format!("json.{}.next", std::process::id()));
    let bytes = serde_json::to_vec(value).map_err(ClosedError::internal)?;
    let mut file = OpenOptions::new()
        .create_new(true)
        .write(true)
        .open(&temporary)
        .map_err(ClosedError::internal)?;
    file.set_permissions(fs::Permissions::from_mode(0o600))
        .map_err(ClosedError::internal)?;
    file.write_all(&bytes)
        .and_then(|()| file.sync_all())
        .map_err(ClosedError::internal)?;
    match fs::rename(&temporary, path) {
        Ok(()) => Ok(()),
        Err(error) => {
            let _ = fs::remove_file(&temporary);
            Err(ClosedError::internal(error))
        }
    }
}

fn sandbox_digest<T: Serialize>(kind: &str, value: &T) -> Result<String> {
    let bytes = serde_json::to_vec(value).map_err(ClosedError::internal)?;
    let mut hash = Sha256::new();
    hash.update(b"praxis.sandbox/v2");
    hash.update([0]);
    hash.update(kind.as_bytes());
    hash.update([0]);
    hash.update(bytes);
    Ok(hex::encode(hash.finalize()))
}

#[allow(dead_code)]
fn _wire_digest_for_review(payload: &WorkspaceCommitPayloadV1) -> Result<String> {
    canonical_digest(
        "ProviderPayloadV1",
        &ProviderPayloadV1::WorkspaceCommit(payload.clone()),
    )
}

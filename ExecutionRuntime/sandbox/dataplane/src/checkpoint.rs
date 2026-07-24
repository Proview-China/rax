use std::collections::BTreeMap;
use std::fs::{self, File, OpenOptions};
use std::io::{Read, Write};
use std::os::unix::fs::{OpenOptionsExt, PermissionsExt};
use std::path::{Component, Path, PathBuf};

use serde::{Deserialize, Serialize};
use sha2::{Digest as _, Sha256};
use tokio::task;

use crate::contract::{DispatchRequestV1, canonical_digest, now_unix_nano};
use crate::error::{ClosedError, ClosedReason, Result};
use crate::provider::{CheckpointArtifactObservationV1, Provider, ProviderResult, provider_result};

pub const CHECKPOINT_EFFECT_KIND_V1: &str = "praxis.sandbox/checkpoint";
const CHECKPOINT_QUERY_VERSION_V1: &str = "praxis.sandbox/checkpoint-current-query/v1";

#[derive(Clone, Copy, Debug, Eq, PartialEq, Serialize, Deserialize)]
#[serde(rename_all = "snake_case")]
pub enum CheckpointParticipantPhaseV1 {
    CheckpointPrepare,
    CheckpointCommit,
    CheckpointAbort,
}

#[derive(Clone, Debug, Eq, PartialEq, Serialize, Deserialize)]
#[serde(deny_unknown_fields)]
pub struct CheckpointExactRefV1 {
    pub id: String,
    pub revision: u64,
    pub digest: String,
    pub expires_unix_nano: i64,
}

impl CheckpointExactRefV1 {
    fn validate_current(&self, name: &str, now: i64) -> Result<()> {
        if self.id.trim().is_empty()
            || self.revision == 0
            || !crate::contract::valid_digest(&self.digest)
            || self.expires_unix_nano <= 0
            || now <= 0
            || now >= self.expires_unix_nano
        {
            return Err(ClosedError::new(
                ClosedReason::CurrentExpired,
                format!("checkpoint {name} exact ref is incomplete or expired"),
            ));
        }
        Ok(())
    }
}

#[derive(Clone, Debug, Eq, PartialEq, Serialize, Deserialize)]
#[serde(deny_unknown_fields)]
pub struct CheckpointPreviousPhaseV1 {
    pub reservation: CheckpointExactRefV1,
    pub closure_id: String,
    pub closure_digest: String,
    pub state: String,
    pub expires_unix_nano: i64,
}

#[derive(Clone, Debug, Eq, PartialEq, Serialize, Deserialize)]
#[serde(deny_unknown_fields)]
pub struct CheckpointRuntimeCurrentQueryV1 {
    pub contract_version: String,
    pub runtime_inspect: serde_json::Value,
    pub phase: CheckpointParticipantPhaseV1,
    pub checkpoint_attempt: CheckpointExactRefV1,
    pub barrier: CheckpointExactRefV1,
    pub effect_cut: CheckpointExactRefV1,
    pub reservation: CheckpointExactRefV1,
    pub participant: CheckpointExactRefV1,
    #[serde(default, skip_serializing_if = "Option::is_none")]
    pub previous_phase: Option<CheckpointPreviousPhaseV1>,
    pub projection_digest: String,
    pub expires_unix_nano: i64,
}

impl CheckpointRuntimeCurrentQueryV1 {
    pub fn from_request(request: &DispatchRequestV1) -> Result<Self> {
        if request.effect_kind != CHECKPOINT_EFFECT_KIND_V1 {
            return Err(ClosedError::new(
                ClosedReason::InvalidArgument,
                "non-checkpoint request cannot carry checkpoint current query",
            ));
        }
        let value: Self =
            serde_json::from_value(request.runtime_current_query.clone()).map_err(|_| {
                ClosedError::new(
                    ClosedReason::InvalidContract,
                    "checkpoint current query shape is invalid",
                )
            })?;
        value.validate_current(request, now_unix_nano())?;
        Ok(value)
    }

    pub fn validate_current(&self, request: &DispatchRequestV1, now: i64) -> Result<()> {
        if self.contract_version != CHECKPOINT_QUERY_VERSION_V1
            || self.runtime_inspect.is_null()
            || !crate::contract::valid_digest(&self.projection_digest)
            || self.expires_unix_nano <= 0
            || now <= 0
            || now >= self.expires_unix_nano
            || self.expires_unix_nano > request.requested_not_after_unix_nano
        {
            return Err(ClosedError::new(
                ClosedReason::CurrentExpired,
                "checkpoint current query is incomplete or expired",
            ));
        }
        self.checkpoint_attempt.validate_current("attempt", now)?;
        self.barrier.validate_current("barrier", now)?;
        self.effect_cut.validate_current("effect cut", now)?;
        self.reservation.validate_current("reservation", now)?;
        self.participant.validate_current("participant", now)?;
        if self.checkpoint_attempt.id.trim().is_empty()
            || self.reservation.id == self.participant.id
        {
            return Err(ClosedError::new(
                ClosedReason::BindingDrift,
                "checkpoint current query identities are spliced",
            ));
        }
        match self.phase {
            CheckpointParticipantPhaseV1::CheckpointPrepare => {
                if self.previous_phase.is_some() {
                    return Err(ClosedError::new(
                        ClosedReason::Conflict,
                        "checkpoint prepare cannot carry a previous phase",
                    ));
                }
            }
            CheckpointParticipantPhaseV1::CheckpointCommit
            | CheckpointParticipantPhaseV1::CheckpointAbort => {
                let previous = self.previous_phase.as_ref().ok_or_else(|| {
                    ClosedError::new(
                        ClosedReason::Conflict,
                        "checkpoint successor requires the exact prepared closure",
                    )
                })?;
                previous
                    .reservation
                    .validate_current("previous reservation", now)?;
                if previous.closure_id.trim().is_empty()
                    || !crate::contract::valid_digest(&previous.closure_digest)
                    || previous.state != "prepared"
                    || previous.expires_unix_nano <= now
                    || previous.expires_unix_nano > previous.reservation.expires_unix_nano
                {
                    return Err(ClosedError::new(
                        ClosedReason::Conflict,
                        "checkpoint successor previous closure is not exact prepared current",
                    ));
                }
            }
        }
        Ok(())
    }

    fn stable_subject(&self, tenant_id: &str) -> String {
        format!(
            "{tenant_id}\0{}\0{}",
            self.checkpoint_attempt.id, self.participant.id
        )
    }
}

#[derive(Clone, Debug, Eq, PartialEq)]
pub enum CheckpointSource {
    Directory(PathBuf),
    File(PathBuf),
}

#[derive(Clone, Debug, Eq, PartialEq, Serialize, Deserialize)]
#[serde(deny_unknown_fields)]
struct CheckpointArtifactRecordV1 {
    contract_version: String,
    subject_digest: String,
    checkpoint_attempt_id: String,
    participant_id: String,
    prepare_reservation_id: String,
    prepare_reservation_revision: u64,
    prepare_reservation_digest: String,
    prepare_reservation_expires_unix_nano: i64,
    source_digest: String,
    content_digest: String,
    content_length: u64,
    state: String,
    operation_digest: String,
    dispatch_attempt_id: String,
    recorded_unix_nano: i64,
    expires_unix_nano: i64,
}

#[derive(Clone, Debug, Eq, PartialEq, Serialize, Deserialize)]
#[serde(deny_unknown_fields)]
struct CheckpointOperationRecordV1 {
    request_digest: String,
    result: ProviderResult,
}

pub struct CheckpointStore {
    root: PathBuf,
}

impl CheckpointStore {
    pub async fn open(root: impl Into<PathBuf>) -> Result<Self> {
        let root = root.into();
        if !root.is_absolute() {
            return Err(ClosedError::new(
                ClosedReason::InvalidArgument,
                "checkpoint store root must be absolute",
            ));
        }
        let root_for_create = root.clone();
        task::spawn_blocking(move || create_secure_directory(&root_for_create))
            .await
            .map_err(ClosedError::internal)??;
        Ok(Self { root })
    }

    pub async fn prepare(
        &self,
        provider: &dyn Provider,
        request: &DispatchRequestV1,
    ) -> Result<ProviderResult> {
        let query = CheckpointRuntimeCurrentQueryV1::from_request(request)?;
        match query.phase {
            CheckpointParticipantPhaseV1::CheckpointPrepare => {
                let source = provider.checkpoint_source(request).await?;
                validate_source(&source).await?;
                provider_result(request, "checkpoint_prepare_validated")
            }
            CheckpointParticipantPhaseV1::CheckpointCommit
            | CheckpointParticipantPhaseV1::CheckpointAbort => {
                let record = self.read_artifact(&query, &request.tenant_id).await?;
                validate_prepared_record(&record, &query, request)?;
                provider_result(
                    request,
                    match query.phase {
                        CheckpointParticipantPhaseV1::CheckpointCommit => {
                            "checkpoint_commit_validated"
                        }
                        CheckpointParticipantPhaseV1::CheckpointAbort => {
                            "checkpoint_abort_validated"
                        }
                        CheckpointParticipantPhaseV1::CheckpointPrepare => unreachable!(),
                    },
                )
            }
        }
    }

    pub async fn execute(
        &self,
        provider: &dyn Provider,
        request: &DispatchRequestV1,
    ) -> Result<ProviderResult> {
        let query = CheckpointRuntimeCurrentQueryV1::from_request(request)?;
        if let Ok(existing) = self.inspect(request).await {
            return Ok(existing);
        }
        let root = self.root.clone();
        let source = if query.phase == CheckpointParticipantPhaseV1::CheckpointPrepare {
            Some(provider.checkpoint_source(request).await?)
        } else {
            None
        };
        let request = request.clone();
        task::spawn_blocking(move || execute_blocking(&root, source, &request, &query))
            .await
            .map_err(ClosedError::internal)?
    }

    pub async fn inspect(&self, request: &DispatchRequestV1) -> Result<ProviderResult> {
        let path = self.operation_path(&request.digest);
        let request = request.clone();
        task::spawn_blocking(move || {
            let bytes = fs::read(path).map_err(|error| {
                if error.kind() == std::io::ErrorKind::NotFound {
                    ClosedError::new(
                        ClosedReason::ProviderUnknown,
                        "checkpoint provider operation has no durable result",
                    )
                } else {
                    ClosedError::internal(error)
                }
            })?;
            let record: CheckpointOperationRecordV1 =
                serde_json::from_slice(&bytes).map_err(|_| {
                    ClosedError::new(
                        ClosedReason::InvalidContract,
                        "checkpoint operation record is invalid",
                    )
                })?;
            if record.request_digest != request.digest {
                return Err(ClosedError::new(
                    ClosedReason::Conflict,
                    "checkpoint operation record binds another request",
                ));
            }
            record
                .result
                .validate(&request, record.result.receipt.recorded_unix_nano)?;
            Ok(record.result)
        })
        .await
        .map_err(ClosedError::internal)?
    }

    async fn read_artifact(
        &self,
        query: &CheckpointRuntimeCurrentQueryV1,
        tenant_id: &str,
    ) -> Result<CheckpointArtifactRecordV1> {
        let path = artifact_record_path(&self.root, query, tenant_id);
        task::spawn_blocking(move || read_artifact_record(&path))
            .await
            .map_err(ClosedError::internal)?
    }

    fn operation_path(&self, request_digest: &str) -> PathBuf {
        self.root
            .join("operations")
            .join(format!("{}.json", digest_path_component(request_digest)))
    }
}

fn execute_blocking(
    root: &Path,
    source: Option<CheckpointSource>,
    request: &DispatchRequestV1,
    query: &CheckpointRuntimeCurrentQueryV1,
) -> Result<ProviderResult> {
    let subject_digest = digest_subject(&query.stable_subject(&request.tenant_id));
    let subject_root = root.join("artifacts").join(&subject_digest);
    create_secure_directory(&subject_root)?;
    let record_path = subject_root.join("current.json");
    let record = match query.phase {
        CheckpointParticipantPhaseV1::CheckpointPrepare => {
            prepare_artifact(source, request, query, &subject_root, &record_path)?
        }
        CheckpointParticipantPhaseV1::CheckpointCommit => {
            commit_artifact(request, query, &subject_root, &record_path)?
        }
        CheckpointParticipantPhaseV1::CheckpointAbort => {
            abort_artifact(request, query, &subject_root, &record_path)?
        }
    };
    let phase = match query.phase {
        CheckpointParticipantPhaseV1::CheckpointPrepare => "checkpoint_prepare",
        CheckpointParticipantPhaseV1::CheckpointCommit => "checkpoint_commit",
        CheckpointParticipantPhaseV1::CheckpointAbort => "checkpoint_abort",
    };
    let result = provider_result(request, &format!("checkpoint_{}", record.state))?
        .with_checkpoint_artifact(CheckpointArtifactObservationV1 {
            contract_version: "praxis.sandbox/checkpoint-artifact-observation/v1".to_owned(),
            artifact_id: format!("praxis-checkpoint:{}", record.subject_digest),
            subject_digest: format!("sha256:{}", record.subject_digest),
            content_digest: record.content_digest.clone(),
            content_length: record.content_length,
            state: record.state.clone(),
            checkpoint_phase: phase.to_owned(),
            recorded_unix_nano: record.recorded_unix_nano,
            expires_unix_nano: record.expires_unix_nano,
        })?;
    let operation = CheckpointOperationRecordV1 {
        request_digest: request.digest.clone(),
        result: result.clone(),
    };
    write_json_atomic(
        &root
            .join("operations")
            .join(format!("{}.json", digest_path_component(&request.digest))),
        &operation,
    )?;
    Ok(result)
}

fn prepare_artifact(
    source: Option<CheckpointSource>,
    request: &DispatchRequestV1,
    query: &CheckpointRuntimeCurrentQueryV1,
    subject_root: &Path,
    record_path: &Path,
) -> Result<CheckpointArtifactRecordV1> {
    if record_path.exists() {
        return Err(ClosedError::new(
            ClosedReason::Conflict,
            "checkpoint artifact subject already has a current record",
        ));
    }
    let source = source.ok_or_else(|| {
        ClosedError::new(ClosedReason::InvalidContract, "checkpoint source is absent")
    })?;
    let staging = subject_root.join("staging");
    let temporary = subject_root.join(format!("staging.tmp.{}", std::process::id()));
    if temporary.exists() {
        fs::remove_dir_all(&temporary).map_err(ClosedError::internal)?;
    }
    create_secure_directory(&temporary)?;
    let (content_digest, content_length) = copy_source_and_digest(&source, &temporary)?;
    fs::rename(&temporary, &staging).map_err(ClosedError::internal)?;
    let now = now_unix_nano();
    let record = CheckpointArtifactRecordV1 {
        contract_version: CHECKPOINT_QUERY_VERSION_V1.to_owned(),
        subject_digest: digest_subject(&query.stable_subject(&request.tenant_id)),
        checkpoint_attempt_id: query.checkpoint_attempt.id.clone(),
        participant_id: query.participant.id.clone(),
        prepare_reservation_id: query.reservation.id.clone(),
        prepare_reservation_revision: query.reservation.revision,
        prepare_reservation_digest: query.reservation.digest.clone(),
        prepare_reservation_expires_unix_nano: query.reservation.expires_unix_nano,
        source_digest: request.payload_digest.clone(),
        content_digest,
        content_length,
        state: "prepared".to_owned(),
        operation_digest: request.operation_digest.clone(),
        dispatch_attempt_id: request.attempt_id.clone(),
        recorded_unix_nano: now,
        expires_unix_nano: checkpoint_write_expiry(request, query),
    };
    write_json_atomic(record_path, &record)?;
    Ok(record)
}

fn commit_artifact(
    request: &DispatchRequestV1,
    query: &CheckpointRuntimeCurrentQueryV1,
    subject_root: &Path,
    record_path: &Path,
) -> Result<CheckpointArtifactRecordV1> {
    let mut record = read_artifact_record(record_path)?;
    validate_prepared_record(&record, query, request)?;
    let staging = subject_root.join("staging");
    let committed = subject_root.join("committed");
    if committed.exists() || !staging.exists() {
        return Err(ClosedError::new(
            ClosedReason::Conflict,
            "checkpoint artifact commit source or target state is invalid",
        ));
    }
    fs::rename(staging, committed).map_err(ClosedError::internal)?;
    update_terminal_record(
        &mut record,
        request,
        "committed",
        now_unix_nano(),
        checkpoint_write_expiry(request, query),
    );
    write_json_atomic(record_path, &record)?;
    Ok(record)
}

fn abort_artifact(
    request: &DispatchRequestV1,
    query: &CheckpointRuntimeCurrentQueryV1,
    subject_root: &Path,
    record_path: &Path,
) -> Result<CheckpointArtifactRecordV1> {
    let mut record = read_artifact_record(record_path)?;
    validate_prepared_record(&record, query, request)?;
    let staging = subject_root.join("staging");
    if staging.exists() {
        fs::remove_dir_all(staging).map_err(ClosedError::internal)?;
    }
    update_terminal_record(
        &mut record,
        request,
        "aborted",
        now_unix_nano(),
        checkpoint_write_expiry(request, query),
    );
    write_json_atomic(record_path, &record)?;
    Ok(record)
}

fn checkpoint_write_expiry(
    request: &DispatchRequestV1,
    query: &CheckpointRuntimeCurrentQueryV1,
) -> i64 {
    request
        .requested_not_after_unix_nano
        .min(request.sandbox_attempt.expires_unix_nano)
        .min(request.execution_binding.expires_unix_nano)
        .min(request.runtime_enforcement.expires_unix_nano)
        .min(query.expires_unix_nano)
}

fn update_terminal_record(
    record: &mut CheckpointArtifactRecordV1,
    request: &DispatchRequestV1,
    state: &str,
    now: i64,
    expires: i64,
) {
    record.state = state.to_owned();
    record
        .operation_digest
        .clone_from(&request.operation_digest);
    record.dispatch_attempt_id.clone_from(&request.attempt_id);
    record.recorded_unix_nano = now;
    record.expires_unix_nano = expires.min(record.expires_unix_nano);
}

fn validate_prepared_record(
    record: &CheckpointArtifactRecordV1,
    query: &CheckpointRuntimeCurrentQueryV1,
    request: &DispatchRequestV1,
) -> Result<()> {
    let previous = query.previous_phase.as_ref().ok_or_else(|| {
        ClosedError::new(
            ClosedReason::Conflict,
            "checkpoint successor has no previous prepared closure",
        )
    })?;
    if record.contract_version != CHECKPOINT_QUERY_VERSION_V1
        || record.state != "prepared"
        || record.checkpoint_attempt_id != query.checkpoint_attempt.id
        || record.participant_id != query.participant.id
        || record.prepare_reservation_id != previous.reservation.id
        || record.prepare_reservation_revision != previous.reservation.revision
        || record.prepare_reservation_digest != previous.reservation.digest
        || record.prepare_reservation_expires_unix_nano != previous.reservation.expires_unix_nano
        || record.source_digest != request.payload_digest
        || record.content_digest.len() != 71
        || !crate::contract::valid_digest(&record.content_digest)
        || record.content_length == 0
        || record.expires_unix_nano <= now_unix_nano()
    {
        return Err(ClosedError::new(
            ClosedReason::Conflict,
            "checkpoint prepared artifact differs from the exact previous phase",
        ));
    }
    Ok(())
}

async fn validate_source(source: &CheckpointSource) -> Result<()> {
    let source = source.clone();
    task::spawn_blocking(move || match source {
        CheckpointSource::Directory(path) => ensure_source_directory(&path),
        CheckpointSource::File(path) => ensure_source_file(&path),
    })
    .await
    .map_err(ClosedError::internal)?
}

fn copy_source_and_digest(source: &CheckpointSource, target: &Path) -> Result<(String, u64)> {
    let mut entries = BTreeMap::<String, String>::new();
    let mut content_length = 0_u64;
    match source {
        CheckpointSource::File(path) => {
            ensure_source_file(path)?;
            let destination = target.join("payload");
            let executable = copy_regular_file(path, &destination)?;
            entries.insert(
                "payload".to_owned(),
                format!(
                    "file:{}:executable={}",
                    sha256_file(path)?,
                    u8::from(executable)
                ),
            );
            content_length = fs::metadata(path).map_err(ClosedError::internal)?.len();
        }
        CheckpointSource::Directory(path) => {
            ensure_source_directory(path)?;
            copy_directory(path, path, target, &mut entries, &mut content_length)?;
        }
    }
    Ok((
        canonical_digest("CheckpointArtifactContentV1", &entries)?,
        content_length,
    ))
}

fn copy_directory(
    root: &Path,
    current: &Path,
    target: &Path,
    entries: &mut BTreeMap<String, String>,
    content_length: &mut u64,
) -> Result<()> {
    let mut children = fs::read_dir(current)
        .map_err(ClosedError::internal)?
        .collect::<std::result::Result<Vec<_>, _>>()
        .map_err(ClosedError::internal)?;
    children.sort_by_key(std::fs::DirEntry::file_name);
    for child in children {
        let source = child.path();
        let metadata = fs::symlink_metadata(&source).map_err(ClosedError::internal)?;
        let relative = source.strip_prefix(root).map_err(ClosedError::internal)?;
        validate_relative_path(relative)?;
        let relative_text = relative.to_string_lossy().replace('\\', "/");
        let destination = target.join(relative);
        if metadata.file_type().is_symlink() {
            return Err(ClosedError::new(
                ClosedReason::InvalidContract,
                "checkpoint source contains a symlink",
            ));
        }
        if metadata.is_dir() {
            create_secure_directory(&destination)?;
            entries.insert(format!("{relative_text}/"), "directory".to_owned());
            copy_directory(root, &source, target, entries, content_length)?;
        } else if metadata.is_file() {
            let executable = copy_regular_file(&source, &destination)?;
            entries.insert(
                relative_text,
                format!(
                    "file:{}:executable={}",
                    sha256_file(&source)?,
                    u8::from(executable)
                ),
            );
            *content_length = content_length.checked_add(metadata.len()).ok_or_else(|| {
                ClosedError::new(
                    ClosedReason::InvalidContract,
                    "checkpoint content length overflow",
                )
            })?;
        } else {
            return Err(ClosedError::new(
                ClosedReason::InvalidContract,
                "checkpoint source contains a special file",
            ));
        }
    }
    Ok(())
}

fn copy_regular_file(source: &Path, destination: &Path) -> Result<bool> {
    if let Some(parent) = destination.parent() {
        create_secure_directory(parent)?;
    }
    let metadata = fs::symlink_metadata(source).map_err(ClosedError::internal)?;
    if !metadata.is_file() || metadata.file_type().is_symlink() {
        return Err(ClosedError::new(
            ClosedReason::InvalidContract,
            "checkpoint source file changed kind during capture",
        ));
    }
    let executable = metadata.permissions().mode() & 0o111 != 0;
    let mut input = OpenOptions::new()
        .read(true)
        .custom_flags(nix::libc::O_NOFOLLOW | nix::libc::O_CLOEXEC)
        .open(source)
        .map_err(ClosedError::internal)?;
    let mut output = OpenOptions::new()
        .write(true)
        .create_new(true)
        .mode(if executable { 0o700 } else { 0o600 })
        .custom_flags(nix::libc::O_NOFOLLOW | nix::libc::O_CLOEXEC)
        .open(destination)
        .map_err(ClosedError::internal)?;
    std::io::copy(&mut input, &mut output).map_err(ClosedError::internal)?;
    output.sync_all().map_err(ClosedError::internal)?;
    Ok(executable)
}

fn sha256_file(path: &Path) -> Result<String> {
    let mut file = File::open(path).map_err(ClosedError::internal)?;
    let mut digest = Sha256::new();
    let mut buffer = vec![0_u8; 64 * 1024];
    loop {
        let read = file.read(&mut buffer).map_err(ClosedError::internal)?;
        if read == 0 {
            break;
        }
        digest.update(&buffer[..read]);
    }
    Ok(format!("sha256:{}", hex::encode(digest.finalize())))
}

fn write_json_atomic<T: Serialize>(path: &Path, value: &T) -> Result<()> {
    let parent = path.parent().ok_or_else(|| {
        ClosedError::new(
            ClosedReason::InvalidArgument,
            "checkpoint record path has no parent",
        )
    })?;
    create_secure_directory(parent)?;
    let temporary = path.with_extension(format!("tmp.{}", std::process::id()));
    let bytes = serde_json::to_vec(value).map_err(ClosedError::internal)?;
    let mut file = OpenOptions::new()
        .write(true)
        .create_new(true)
        .mode(0o600)
        .custom_flags(nix::libc::O_NOFOLLOW | nix::libc::O_CLOEXEC)
        .open(&temporary)
        .map_err(ClosedError::internal)?;
    file.write_all(&bytes).map_err(ClosedError::internal)?;
    file.sync_all().map_err(ClosedError::internal)?;
    fs::rename(&temporary, path).map_err(ClosedError::internal)?;
    File::open(parent)
        .and_then(|directory| directory.sync_all())
        .map_err(ClosedError::internal)
}

fn read_artifact_record(path: &Path) -> Result<CheckpointArtifactRecordV1> {
    let bytes = fs::read(path).map_err(|error| {
        if error.kind() == std::io::ErrorKind::NotFound {
            ClosedError::new(
                ClosedReason::NotFoundObservation,
                "checkpoint prepared artifact is not found",
            )
        } else {
            ClosedError::internal(error)
        }
    })?;
    serde_json::from_slice(&bytes).map_err(|_| {
        ClosedError::new(
            ClosedReason::InvalidContract,
            "checkpoint artifact record is invalid",
        )
    })
}

fn artifact_record_path(
    root: &Path,
    query: &CheckpointRuntimeCurrentQueryV1,
    tenant_id: &str,
) -> PathBuf {
    root.join("artifacts")
        .join(digest_subject(&query.stable_subject(tenant_id)))
        .join("current.json")
}

fn digest_subject(value: &str) -> String {
    let mut digest = Sha256::new();
    digest.update(value.as_bytes());
    hex::encode(digest.finalize())
}

fn digest_path_component(value: &str) -> String {
    value.strip_prefix("sha256:").unwrap_or(value).to_owned()
}

fn ensure_source_directory(path: &Path) -> Result<()> {
    let metadata = fs::symlink_metadata(path).map_err(ClosedError::internal)?;
    if !path.is_absolute() || !metadata.is_dir() || metadata.file_type().is_symlink() {
        return Err(ClosedError::new(
            ClosedReason::InvalidContract,
            "checkpoint source is not an absolute real directory",
        ));
    }
    Ok(())
}

fn ensure_source_file(path: &Path) -> Result<()> {
    let metadata = fs::symlink_metadata(path).map_err(ClosedError::internal)?;
    if !path.is_absolute() || !metadata.is_file() || metadata.file_type().is_symlink() {
        return Err(ClosedError::new(
            ClosedReason::InvalidContract,
            "checkpoint source is not an absolute regular file",
        ));
    }
    Ok(())
}

fn validate_relative_path(path: &Path) -> Result<()> {
    if path.as_os_str().is_empty()
        || path
            .components()
            .any(|component| !matches!(component, Component::Normal(_)))
    {
        return Err(ClosedError::new(
            ClosedReason::InvalidContract,
            "checkpoint source path is not canonical relative",
        ));
    }
    Ok(())
}

fn create_secure_directory(path: &Path) -> Result<()> {
    fs::create_dir_all(path).map_err(ClosedError::internal)?;
    fs::set_permissions(path, fs::Permissions::from_mode(0o700)).map_err(ClosedError::internal)
}

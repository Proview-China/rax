use std::collections::BTreeMap;
use std::fs::File;
use std::os::unix::fs::{FileExt as _, MetadataExt as _};
use std::path::{Path, PathBuf};
use std::sync::atomic::{AtomicU64, Ordering};

use async_trait::async_trait;
use nix::fcntl::{OFlag, OpenHow, ResolveFlag, open, openat2};
use nix::sys::stat::Mode;
use serde::Deserialize;
use sha2::{Digest as _, Sha256};

use crate::contract::{
    DispatchRequestV1, ProviderKindV1, ProviderPayloadV1, WorkspaceReadPayloadV1,
};
use crate::error::{ClosedError, ClosedReason, EffectBoundary, Result};
use crate::provider::{Provider, ProviderResult, WorkspaceReadObservationV1, provider_result};

pub const MAX_WORKSPACE_READ_BYTES_V1: u64 = 1_048_576;

#[derive(Clone, Debug, Deserialize)]
#[serde(deny_unknown_fields)]
pub struct WorkspaceReadBindingV1 {
    pub path: PathBuf,
    pub digest: String,
}

#[derive(Clone, Debug, Deserialize)]
#[serde(deny_unknown_fields)]
pub struct WorkspaceReadConfigV1 {
    pub bindings: BTreeMap<String, WorkspaceReadBindingV1>,
}

pub struct WorkspaceReadProviderV1 {
    config: WorkspaceReadConfigV1,
    physical_reads: AtomicU64,
}

impl WorkspaceReadProviderV1 {
    #[must_use]
    pub fn new(config: WorkspaceReadConfigV1) -> Self {
        Self {
            config,
            physical_reads: AtomicU64::new(0),
        }
    }

    pub fn probe(&self) -> Result<()> {
        if self.config.bindings.is_empty() {
            return Err(ClosedError::new(
                ClosedReason::InvalidContract,
                "workspace read requires at least one exact binding",
            ));
        }
        for binding in self.config.bindings.values() {
            if !binding.path.is_absolute()
                || !valid_digest(&binding.digest)
                || !binding.path.is_dir()
            {
                return Err(ClosedError::new(
                    ClosedReason::InvalidContract,
                    "workspace read binding is invalid",
                ));
            }
        }
        Ok(())
    }

    #[must_use]
    pub fn physical_read_count(&self) -> u64 {
        self.physical_reads.load(Ordering::SeqCst)
    }

    fn payload<'a>(
        &'a self,
        request: &'a DispatchRequestV1,
    ) -> Result<(&'a WorkspaceReadPayloadV1, &'a WorkspaceReadBindingV1)> {
        let ProviderPayloadV1::WorkspaceRead(payload) = &request.payload else {
            return Err(ClosedError::new(
                ClosedReason::BindingDrift,
                "workspace read Provider received another payload",
            ));
        };
        let binding = self
            .config
            .bindings
            .get(&payload.workspace_binding_id)
            .ok_or_else(|| {
                ClosedError::new(
                    ClosedReason::NotFoundObservation,
                    "workspace read binding was not found",
                )
            })?;
        if binding.digest != payload.workspace_digest {
            return Err(ClosedError::new(
                ClosedReason::InvalidDigest,
                "workspace read binding digest drifted",
            ));
        }
        Ok((payload, binding))
    }

    fn execute_read(&self, request: &DispatchRequestV1) -> Result<ProviderResult> {
        let actual_point = AtomicU64::new(0);
        self.execute_read_inner(request, &actual_point)
            .map_err(|error| {
                let physical_read_count = actual_point.load(Ordering::SeqCst);
                let crossed = physical_read_count != 0;
                error
                    .with_effect_boundary(if crossed {
                        EffectBoundary::EffectStartedUnknown
                    } else {
                        EffectBoundary::EffectNotStarted
                    })
                    .with_actual_point_evidence(crossed, physical_read_count)
            })
    }

    fn execute_read_inner(
        &self,
        request: &DispatchRequestV1,
        actual_point: &AtomicU64,
    ) -> Result<ProviderResult> {
        let (payload, binding) = self.payload(request)?;
        let full = read_exact_utf8_beneath(
            &binding.path,
            &payload.relative_path,
            payload.start_byte,
            || {
                actual_point.store(1, Ordering::SeqCst);
                self.physical_reads.fetch_add(1, Ordering::SeqCst);
            },
        )?;
        let total_bytes = u64::try_from(full.len()).map_err(ClosedError::internal)?;
        let end = total_bytes.min(payload.start_byte.saturating_add(payload.max_bytes));
        let start = usize::try_from(payload.start_byte).map_err(ClosedError::internal)?;
        let end_usize = usize::try_from(end).map_err(ClosedError::internal)?;
        let content =
            String::from_utf8(full.as_bytes()[start..end_usize].to_vec()).map_err(|_| {
                ClosedError::new(
                    ClosedReason::InvalidArgument,
                    "workspace read range splits a UTF-8 code point",
                )
            })?;
        let content_digest = workspace_read_content_digest(
            content.as_bytes(),
            payload.start_byte,
            total_bytes,
            end == total_bytes,
        );
        let file_digest = format!("sha256:{}", hex::encode(Sha256::digest(full.as_bytes())));
        let expires = request
            .requested_not_after_unix_nano
            .min(request.sandbox_attempt.expires_unix_nano)
            .min(request.execution_binding.expires_unix_nano)
            .min(request.runtime_enforcement.expires_unix_nano)
            .min(payload.workspace.expires_unix_nano);
        let file_id_digest = hex::encode(Sha256::digest(
            format!("{}\0{}", payload.workspace.id, payload.relative_path).as_bytes(),
        ));
        let file = crate::contract::ExactRefV1 {
            id: format!("workspace-file-{file_id_digest}"),
            revision: payload.workspace.revision,
            digest: file_digest,
            expires_unix_nano: payload.workspace.expires_unix_nano,
        };
        if payload
            .expected_file_ref
            .as_ref()
            .is_some_and(|expected| expected != &file)
        {
            return Err(ClosedError::new(
                ClosedReason::BindingDrift,
                "workspace read expected file ref drifted",
            ));
        }
        provider_result(request, "workspace_read_observed")?.with_workspace_read(
            WorkspaceReadObservationV1 {
                contract_version: "praxis.sandbox/workspace-read-observation/v1".to_owned(),
                workspace: payload.workspace.clone(),
                file,
                relative_path: payload.relative_path.clone(),
                start_byte: payload.start_byte,
                returned_bytes: u64::try_from(content.len()).map_err(ClosedError::internal)?,
                total_bytes,
                complete: end == total_bytes,
                content,
                content_digest,
                s1_checked: payload.s1_checked,
                s2_checked: true,
                physical_read_count: 1,
                recorded_unix_nano: 0,
                expires_unix_nano: expires,
            },
        )
    }
}

#[async_trait]
impl Provider for WorkspaceReadProviderV1 {
    fn kind(&self) -> ProviderKindV1 {
        ProviderKindV1::WorkspaceRead
    }
    async fn prepare(&self, request: &DispatchRequestV1) -> Result<ProviderResult> {
        self.payload(request)?;
        provider_result(request, "workspace_read_prepared")
    }
    async fn execute_prepared(&self, request: &DispatchRequestV1) -> Result<ProviderResult> {
        self.execute_read(request)
    }
    async fn inspect(&self, _: &DispatchRequestV1) -> Result<ProviderResult> {
        Err(ClosedError::new(
            ClosedReason::ProviderUnknown,
            "inspect the exact original attempt through the durable journal",
        ))
    }
    async fn fence(&self, _: &DispatchRequestV1) -> Result<ProviderResult> {
        Err(ClosedError::new(
            ClosedReason::Unsupported,
            "workspace read cannot be fenced",
        ))
    }
    async fn release(&self, _: &DispatchRequestV1) -> Result<ProviderResult> {
        Err(ClosedError::new(
            ClosedReason::Unsupported,
            "workspace read has no release action",
        ))
    }
    async fn cleanup(&self, _: &DispatchRequestV1) -> Result<ProviderResult> {
        Err(ClosedError::new(
            ClosedReason::Unsupported,
            "workspace read has no cleanup action",
        ))
    }
    async fn inspect_cleanup(&self, _: &DispatchRequestV1) -> Result<ProviderResult> {
        Err(ClosedError::new(
            ClosedReason::Unsupported,
            "workspace read has no cleanup observation",
        ))
    }
}

#[cfg(target_os = "linux")]
fn read_exact_utf8_beneath<F: FnOnce()>(
    root: &Path,
    relative: &str,
    start_byte: u64,
    on_actual_point: F,
) -> Result<String> {
    read_exact_utf8_beneath_with_hooks(root, relative, start_byte, on_actual_point, || {})
}

#[cfg(target_os = "linux")]
fn read_exact_utf8_beneath_with_hooks<F: FnOnce(), G: FnOnce()>(
    root: &Path,
    relative: &str,
    start_byte: u64,
    on_actual_point: F,
    after_read: G,
) -> Result<String> {
    if !valid_logical_path(relative) {
        return Err(ClosedError::new(
            ClosedReason::InvalidArgument,
            "workspace read coordinates violate the closed bound",
        ));
    }
    let root_fd = open(
        root,
        OFlag::O_RDONLY | OFlag::O_DIRECTORY | OFlag::O_CLOEXEC | OFlag::O_NOFOLLOW,
        Mode::empty(),
    )
    .map_err(map_open_error)?;
    let how = OpenHow::new()
        .flags(OFlag::O_RDONLY | OFlag::O_CLOEXEC | OFlag::O_NOFOLLOW)
        .resolve(
            ResolveFlag::RESOLVE_BENEATH
                | ResolveFlag::RESOLVE_NO_MAGICLINKS
                | ResolveFlag::RESOLVE_NO_SYMLINKS,
        );
    let fd = openat2(&root_fd, relative, how).map_err(map_open_error)?;
    let file = File::from(fd);
    let metadata_before = file.metadata().map_err(ClosedError::internal)?;
    if !metadata_before.is_file() {
        return Err(ClosedError::new(
            ClosedReason::InvalidArgument,
            "workspace read target is not a regular file",
        ));
    }
    if metadata_before.len() > MAX_WORKSPACE_READ_BYTES_V1 {
        return Err(ClosedError::new(
            ClosedReason::FrameTooLarge,
            "workspace read target exceeds the 1 MiB file bound",
        ));
    }
    if start_byte > metadata_before.len() {
        return Err(ClosedError::new(
            ClosedReason::InvalidArgument,
            "workspace read start_byte is beyond EOF",
        ));
    }
    let expected = usize::try_from(metadata_before.len()).map_err(ClosedError::internal)?;
    let mut bytes = vec![0_u8; expected];
    let mut offset = 0;
    on_actual_point();
    while offset < expected {
        let read = file
            .read_at(
                &mut bytes[offset..],
                u64::try_from(offset).map_err(ClosedError::internal)?,
            )
            .map_err(ClosedError::internal)?;
        if read == 0 {
            return Err(ClosedError::new(
                ClosedReason::ProviderUnknown,
                "workspace file changed during pread",
            ));
        }
        offset += read;
    }
    let mut extra = [0_u8; 1];
    if file
        .read_at(&mut extra, metadata_before.len())
        .map_err(ClosedError::internal)?
        != 0
    {
        return Err(ClosedError::new(
            ClosedReason::ProviderUnknown,
            "workspace file grew during pread",
        ));
    }
    after_read();
    let metadata_after = file.metadata().map_err(ClosedError::internal)?;
    let second_fd = openat2(&root_fd, relative, how).map_err(map_open_error)?;
    let second = File::from(second_fd);
    let metadata_reopened = second.metadata().map_err(ClosedError::internal)?;
    if !same_file_identity(&metadata_before, &metadata_after)
        || !same_file_identity(&metadata_before, &metadata_reopened)
    {
        return Err(ClosedError::new(
            ClosedReason::ProviderUnknown,
            "workspace file identity changed across the actual point",
        ));
    }
    String::from_utf8(bytes).map_err(|_| {
        ClosedError::new(
            ClosedReason::InvalidArgument,
            "workspace read target is not complete UTF-8",
        )
    })
}

#[cfg(target_os = "linux")]
fn same_file_identity(left: &std::fs::Metadata, right: &std::fs::Metadata) -> bool {
    left.is_file()
        && right.is_file()
        && left.dev() == right.dev()
        && left.ino() == right.ino()
        && left.len() == right.len()
        && left.mtime() == right.mtime()
        && left.mtime_nsec() == right.mtime_nsec()
        && left.ctime() == right.ctime()
        && left.ctime_nsec() == right.ctime_nsec()
}

fn workspace_read_content_digest(content: &[u8], start: u64, total: u64, complete: bool) -> String {
    let mut hash = Sha256::new();
    hash.update(b"praxis.sandbox/workspace-read-range/v1");
    hash.update([0]);
    hash.update(start.to_string().as_bytes());
    hash.update([0]);
    hash.update(total.to_string().as_bytes());
    hash.update([0]);
    hash.update(if complete {
        &b"true"[..]
    } else {
        &b"false"[..]
    });
    hash.update([0]);
    hash.update(content);
    format!("sha256:{}", hex::encode(hash.finalize()))
}

#[cfg(not(target_os = "linux"))]
fn read_exact_utf8_beneath<F: FnOnce()>(_: &Path, _: &str, _: u64, _: F) -> Result<String> {
    Err(ClosedError::new(
        ClosedReason::Unsupported,
        "workspace read v1 requires Linux openat2",
    ))
}

fn map_open_error(error: nix::errno::Errno) -> ClosedError {
    match error {
        nix::errno::Errno::ENOENT => ClosedError::new(
            ClosedReason::NotFoundObservation,
            "workspace read target was not found",
        ),
        nix::errno::Errno::ELOOP | nix::errno::Errno::EXDEV => ClosedError::new(
            ClosedReason::BindingDrift,
            "workspace read path escaped or traversed a symlink",
        ),
        _ => ClosedError::new(
            ClosedReason::ProviderUnavailable,
            "workspace read openat2 failed",
        ),
    }
}
fn valid_logical_path(value: &str) -> bool {
    !value.is_empty()
        && !value.starts_with('/')
        && !value.contains('\\')
        && !value
            .split('/')
            .any(|part| part.is_empty() || part == "." || part == "..")
}
fn valid_digest(value: &str) -> bool {
    value.len() == 71
        && value.starts_with("sha256:")
        && value[7..].bytes().all(|byte| byte.is_ascii_hexdigit())
}

#[cfg(all(test, target_os = "linux"))]
#[allow(clippy::expect_used, clippy::unwrap_used)]
mod tests {
    use super::*;
    use std::fs;

    #[test]
    fn same_inode_modification_after_pread_is_indeterminate() {
        let temporary = tempfile::tempdir().unwrap();
        fs::write(temporary.path().join("file.txt"), "before").unwrap();
        let reads = AtomicU64::new(0);
        let error = read_exact_utf8_beneath_with_hooks(
            temporary.path(),
            "file.txt",
            0,
            || {
                reads.fetch_add(1, Ordering::SeqCst);
            },
            || {
                fs::write(temporary.path().join("file.txt"), "after!").unwrap();
            },
        )
        .expect_err("same inode mutation must not return content");
        assert_eq!(error.reason, ClosedReason::ProviderUnknown);
        assert_eq!(reads.load(Ordering::SeqCst), 1);
    }

    #[test]
    fn path_replacement_after_pread_is_indeterminate() {
        let temporary = tempfile::tempdir().unwrap();
        fs::write(temporary.path().join("file.txt"), "before").unwrap();
        let reads = AtomicU64::new(0);
        let error = read_exact_utf8_beneath_with_hooks(
            temporary.path(),
            "file.txt",
            0,
            || {
                reads.fetch_add(1, Ordering::SeqCst);
            },
            || {
                fs::rename(
                    temporary.path().join("file.txt"),
                    temporary.path().join("old.txt"),
                )
                .unwrap();
                fs::write(temporary.path().join("file.txt"), "before").unwrap();
            },
        )
        .expect_err("path replacement must not return content");
        assert_eq!(error.reason, ClosedReason::ProviderUnknown);
        assert_eq!(reads.load(Ordering::SeqCst), 1);
    }
}

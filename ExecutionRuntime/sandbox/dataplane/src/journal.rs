use std::collections::BTreeMap;
use std::path::{Path, PathBuf};

use serde::{Deserialize, Serialize};
use tokio::fs::{self, OpenOptions};
use tokio::io::AsyncWriteExt;
use tokio::sync::Mutex;

use crate::contract::{
    DispatchRequestV1, EnforcementPhaseV1, canonical_digest, now_unix_nano, valid_digest,
};
use crate::error::{ClosedError, ClosedReason, Result};
use crate::provider::ProviderResult;

#[derive(Clone, Copy, Debug, Eq, PartialEq, Serialize, Deserialize)]
#[serde(rename_all = "snake_case")]
enum JournalState {
    Started,
    Completed,
}

#[derive(Clone, Debug, Eq, PartialEq, Serialize, Deserialize)]
#[serde(deny_unknown_fields)]
pub struct WorkspaceReadPhysicalJournalRefV2 {
    pub attempt_id: String,
    pub request_digest: String,
    pub payload_digest: String,
    pub phase: EnforcementPhaseV1,
    pub state: String,
    pub revision: u64,
    pub recorded_unix_nano: i64,
    pub record_digest: String,
}

impl WorkspaceReadPhysicalJournalRefV2 {
    pub fn validate(&self, request: &DispatchRequestV1) -> Result<()> {
        if self.attempt_id != request.attempt_id
            || self.request_digest != request.digest
            || self.payload_digest != request.payload_digest
            || self.phase != request.phase
            || self.recorded_unix_nano <= 0
            || !matches!(
                (self.state.as_str(), self.revision),
                ("started", 1) | ("completed", 2)
            )
            || self.record_digest != self.calculate_digest()?
        {
            return Err(ClosedError::new(
                ClosedReason::BindingDrift,
                "workspace read physical journal reference drifted",
            ));
        }
        Ok(())
    }

    fn calculate_digest(&self) -> Result<String> {
        let mut canonical = self.clone();
        canonical.record_digest.clear();
        canonical_digest("WorkspaceReadPhysicalJournalRefV2", &canonical)
    }

    pub fn validate_lookup(&self, lookup: &WorkspaceReadPhysicalJournalLookupV2) -> Result<()> {
        lookup.validate()?;
        if self.attempt_id != lookup.attempt_id
            || self.request_digest != lookup.request_digest
            || self.payload_digest != lookup.payload_digest
            || self.phase != lookup.phase
            || self.recorded_unix_nano <= 0
            || !matches!(
                (self.state.as_str(), self.revision),
                ("started", 1) | ("completed", 2)
            )
            || self.record_digest != self.calculate_digest()?
        {
            return Err(ClosedError::new(
                ClosedReason::BindingDrift,
                "workspace read physical journal reference drifted from exact lookup",
            ));
        }
        Ok(())
    }
}

#[derive(Clone, Debug, Eq, PartialEq)]
pub struct WorkspaceReadPhysicalJournalInspectionV2 {
    pub journal: WorkspaceReadPhysicalJournalRefV2,
    pub request: Option<DispatchRequestV1>,
    pub result: Option<ProviderResult>,
}

#[derive(Clone, Debug, Eq, PartialEq, Serialize, Deserialize)]
#[serde(deny_unknown_fields)]
pub struct WorkspaceReadPhysicalJournalLookupV2 {
    pub attempt_id: String,
    pub request_digest: String,
    pub payload_digest: String,
    pub phase: EnforcementPhaseV1,
    pub digest: String,
}

impl WorkspaceReadPhysicalJournalLookupV2 {
    pub fn validate(&self) -> Result<()> {
        if self.attempt_id.trim().is_empty()
            || !valid_digest(&self.request_digest)
            || !valid_digest(&self.payload_digest)
            || self.phase != EnforcementPhaseV1::Execute
            || self.digest != self.calculate_digest()?
        {
            return Err(ClosedError::new(
                ClosedReason::InvalidContract,
                "workspace read journal lookup is incomplete",
            ));
        }
        Ok(())
    }

    fn calculate_digest(&self) -> Result<String> {
        let mut canonical = self.clone();
        canonical.digest.clear();
        canonical_digest("WorkspaceReadPhysicalJournalLookupV2", &canonical)
    }
}

#[derive(Clone, Debug, Eq, PartialEq, Serialize, Deserialize)]
#[serde(deny_unknown_fields)]
struct JournalRecord {
    attempt_id: String,
    request_digest: String,
    payload_digest: String,
    phase: EnforcementPhaseV1,
    state: JournalState,
    recorded_unix_nano: i64,
    #[serde(default, skip_serializing_if = "Option::is_none")]
    request: Option<DispatchRequestV1>,
    #[serde(default, skip_serializing_if = "Option::is_none")]
    result: Option<ProviderResult>,
}

#[derive(Default)]
struct JournalCurrent {
    requests: BTreeMap<String, JournalRecord>,
    prepared_payloads: BTreeMap<String, String>,
}

/// Local crash boundary. It never grants Runtime authority; it only prevents a
/// provider call from being repeated after `Started` has been made durable.
pub struct AttemptJournal {
    path: PathBuf,
    current: Mutex<JournalCurrent>,
}

impl AttemptJournal {
    pub async fn open(path: impl AsRef<Path>) -> Result<Self> {
        let path = path.as_ref().to_path_buf();
        if let Some(parent) = path.parent() {
            fs::create_dir_all(parent)
                .await
                .map_err(ClosedError::internal)?;
        }
        let mut current = JournalCurrent::default();
        match fs::read_to_string(&path).await {
            Ok(contents) => {
                for line in contents.lines().filter(|line| !line.trim().is_empty()) {
                    let record: JournalRecord = serde_json::from_str(line).map_err(|_| {
                        ClosedError::new(
                            ClosedReason::InvalidContract,
                            "provider journal contains an invalid record",
                        )
                    })?;
                    apply_record(&mut current, record)?;
                }
            }
            Err(error) if error.kind() == std::io::ErrorKind::NotFound => {}
            Err(error) => return Err(ClosedError::internal(error)),
        }
        Ok(Self {
            path,
            current: Mutex::new(current),
        })
    }

    pub async fn begin(&self, request: &DispatchRequestV1) -> Result<()> {
        let mut current = self.current.lock().await;
        if request.phase == EnforcementPhaseV1::Execute
            && current.prepared_payloads.get(&request.attempt_id) != Some(&request.payload_digest)
        {
            return Err(ClosedError::new(
                ClosedReason::Conflict,
                "execute requires the same attempt and payload to have completed prepare",
            ));
        }
        if let Some(existing) = current.requests.get(&request.digest) {
            let (reason, message) = match existing.state {
                JournalState::Started => (
                    ClosedReason::ProviderUnknown,
                    "provider attempt already began; inspect the original attempt",
                ),
                JournalState::Completed => (
                    ClosedReason::Conflict,
                    "provider attempt already completed; inspect instead of executing again",
                ),
            };
            return Err(ClosedError::new(reason, message));
        }
        let record = JournalRecord {
            attempt_id: request.attempt_id.clone(),
            request_digest: request.digest.clone(),
            payload_digest: request.payload_digest.clone(),
            phase: request.phase,
            state: JournalState::Started,
            recorded_unix_nano: now_unix_nano(),
            request: Some(request.clone()),
            result: None,
        };
        append_record(&self.path, &record).await?;
        apply_record(&mut current, record)
    }

    pub async fn complete(
        &self,
        request: &DispatchRequestV1,
        result: &ProviderResult,
    ) -> Result<()> {
        result.validate(request, now_unix_nano())?;
        let mut current = self.current.lock().await;
        let Some(existing) = current.requests.get(&request.digest) else {
            return Err(ClosedError::new(
                ClosedReason::Conflict,
                "provider completion has no durable begin",
            ));
        };
        if existing.state != JournalState::Started {
            return Err(ClosedError::new(
                ClosedReason::Conflict,
                "provider completion was already recorded",
            ));
        }
        let record = JournalRecord {
            attempt_id: request.attempt_id.clone(),
            request_digest: request.digest.clone(),
            payload_digest: request.payload_digest.clone(),
            phase: request.phase,
            state: JournalState::Completed,
            recorded_unix_nano: now_unix_nano(),
            request: Some(request.clone()),
            result: Some(result.clone()),
        };
        append_record(&self.path, &record).await?;
        apply_record(&mut current, record)
    }

    /// Returns only the durable result for the exact original request. It
    /// never reads Runtime current facts and never calls a Provider.
    pub async fn inspect(&self, request: &DispatchRequestV1) -> Result<ProviderResult> {
        let inspection = self.inspect_workspace_read_v2(request).await?;
        inspection.result.ok_or_else(|| {
            ClosedError::new(
                ClosedReason::ProviderUnknown,
                "provider attempt began but has no durable result",
            )
        })
    }

    /// Returns the exact append-only journal evidence for the original request.
    /// This is a historical read: it validates request shape and exact identity,
    /// but never validates current TTL and never calls a Provider.
    pub async fn inspect_workspace_read_v2(
        &self,
        request: &DispatchRequestV1,
    ) -> Result<WorkspaceReadPhysicalJournalInspectionV2> {
        request.validate_shape()?;
        let current = self.current.lock().await;
        let Some(record) = current.requests.get(&request.digest) else {
            return Err(ClosedError::new(
                ClosedReason::ProviderUnknown,
                "provider attempt is absent from the durable journal",
            ));
        };
        if record.attempt_id != request.attempt_id
            || record.payload_digest != request.payload_digest
            || record.phase != request.phase
        {
            return Err(ClosedError::new(
                ClosedReason::Conflict,
                "provider inspect coordinates drifted from the original attempt",
            ));
        }
        let state = match record.state {
            JournalState::Started => "started",
            JournalState::Completed => "completed",
        };
        let mut journal = WorkspaceReadPhysicalJournalRefV2 {
            attempt_id: record.attempt_id.clone(),
            request_digest: record.request_digest.clone(),
            payload_digest: record.payload_digest.clone(),
            phase: record.phase,
            state: state.to_owned(),
            revision: if record.state == JournalState::Started {
                1
            } else {
                2
            },
            recorded_unix_nano: record.recorded_unix_nano,
            record_digest: String::new(),
        };
        journal.record_digest = journal.calculate_digest()?;
        journal.validate(request)?;
        let result = record.result.clone();
        match (&record.state, &result) {
            (JournalState::Started, None) => {}
            (JournalState::Completed, Some(result)) => {
                result.validate(request, result.receipt.recorded_unix_nano)?;
            }
            _ => {
                return Err(ClosedError::new(
                    ClosedReason::InvalidContract,
                    "provider journal result presence does not match its state",
                ));
            }
        }
        Ok(WorkspaceReadPhysicalJournalInspectionV2 {
            journal,
            request: record.request.clone(),
            result,
        })
    }

    pub async fn inspect_workspace_read_lookup_v2(
        &self,
        lookup: &WorkspaceReadPhysicalJournalLookupV2,
    ) -> Result<WorkspaceReadPhysicalJournalInspectionV2> {
        lookup.validate()?;
        let current = self.current.lock().await;
        let Some(record) = current.requests.get(&lookup.request_digest) else {
            return Err(ClosedError::new(
                ClosedReason::ProviderUnknown,
                "workspace read journal lookup has no exact record",
            ));
        };
        if record.attempt_id != lookup.attempt_id
            || record.payload_digest != lookup.payload_digest
            || record.phase != lookup.phase
        {
            return Err(ClosedError::new(
                ClosedReason::Conflict,
                "workspace read journal lookup drifted from stored axes",
            ));
        }
        let state = match record.state {
            JournalState::Started => "started",
            JournalState::Completed => "completed",
        };
        let mut journal = WorkspaceReadPhysicalJournalRefV2 {
            attempt_id: record.attempt_id.clone(),
            request_digest: record.request_digest.clone(),
            payload_digest: record.payload_digest.clone(),
            phase: record.phase,
            state: state.to_owned(),
            revision: if record.state == JournalState::Started {
                1
            } else {
                2
            },
            recorded_unix_nano: record.recorded_unix_nano,
            record_digest: String::new(),
        };
        journal.record_digest = journal.calculate_digest()?;
        journal.validate_lookup(lookup)?;
        let (request, result) = match (&record.request, &record.result) {
            (Some(request), Some(result)) => {
                request.validate_shape()?;
                if request.attempt_id != lookup.attempt_id
                    || request.digest != lookup.request_digest
                    || request.payload_digest != lookup.payload_digest
                    || request.phase != lookup.phase
                {
                    return Err(ClosedError::new(
                        ClosedReason::Conflict,
                        "workspace read stored request drifted from journal lookup",
                    ));
                }
                result.validate(request, result.receipt.recorded_unix_nano)?;
                (Some(request.clone()), Some(result.clone()))
            }
            (Some(request), None) => {
                request.validate_shape()?;
                if request.attempt_id != lookup.attempt_id
                    || request.digest != lookup.request_digest
                    || request.payload_digest != lookup.payload_digest
                    || request.phase != lookup.phase
                {
                    return Err(ClosedError::new(
                        ClosedReason::Conflict,
                        "workspace read stored request drifted from started journal lookup",
                    ));
                }
                (Some(request.clone()), None)
            }
            (None, None | Some(_)) => (None, None),
            // Legacy completed rows prove the physical journal state but lack
            // the sealed request body required to re-materialize Observed.
            // They therefore remain evidence-only and become Indeterminate.
        };
        Ok(WorkspaceReadPhysicalJournalInspectionV2 {
            journal,
            request,
            result,
        })
    }
}

fn apply_record(current: &mut JournalCurrent, record: JournalRecord) -> Result<()> {
    if let Some(previous) = current.requests.get(&record.request_digest)
        && (previous.attempt_id != record.attempt_id
            || previous.payload_digest != record.payload_digest
            || previous.phase != record.phase
            || previous.state == JournalState::Completed
            || previous.request != record.request
            || record.state != JournalState::Completed)
    {
        return Err(ClosedError::new(
            ClosedReason::Conflict,
            "provider journal violates append-only phase ordering",
        ));
    }
    if (record.state == JournalState::Started) != record.result.is_none() {
        return Err(ClosedError::new(
            ClosedReason::InvalidContract,
            "provider journal result presence does not match its state",
        ));
    }
    if record.attempt_id.trim().is_empty()
        || !valid_digest(&record.request_digest)
        || !valid_digest(&record.payload_digest)
        || record.recorded_unix_nano <= 0
    {
        return Err(ClosedError::new(
            ClosedReason::InvalidContract,
            "provider journal exact axes are incomplete",
        ));
    }
    if let Some(request) = &record.request {
        request.validate_shape()?;
        if request.attempt_id != record.attempt_id
            || request.digest != record.request_digest
            || request.payload_digest != record.payload_digest
            || request.phase != record.phase
        {
            return Err(ClosedError::new(
                ClosedReason::BindingDrift,
                "provider journal request body drifted from its exact axes",
            ));
        }
    }
    if record.state == JournalState::Completed && record.phase == EnforcementPhaseV1::Prepare {
        match current.prepared_payloads.get(&record.attempt_id) {
            Some(digest) if digest != &record.payload_digest => {
                return Err(ClosedError::new(
                    ClosedReason::Conflict,
                    "attempt prepare payload changed",
                ));
            }
            Some(_) => {}
            None => {
                current
                    .prepared_payloads
                    .insert(record.attempt_id.clone(), record.payload_digest.clone());
            }
        }
    }
    current
        .requests
        .insert(record.request_digest.clone(), record);
    Ok(())
}

async fn append_record(path: &Path, record: &JournalRecord) -> Result<()> {
    let mut bytes = serde_json::to_vec(record).map_err(ClosedError::internal)?;
    bytes.push(b'\n');
    let mut file = OpenOptions::new()
        .create(true)
        .append(true)
        .open(path)
        .await
        .map_err(ClosedError::internal)?;
    file.write_all(&bytes)
        .await
        .map_err(ClosedError::internal)?;
    file.sync_data().await.map_err(ClosedError::internal)
}

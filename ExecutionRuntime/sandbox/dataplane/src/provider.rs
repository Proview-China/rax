use async_trait::async_trait;
use serde::{Deserialize, Serialize};

use crate::checkpoint::CheckpointSource;
use crate::contract::{
    DispatchRequestV1, EnforcementPhaseV1, ExactRefV1, ProviderKindV1, canonical_digest,
    now_unix_nano,
};
use crate::error::{ClosedError, ClosedReason, Result};

#[derive(Clone, Debug, Eq, PartialEq, Serialize, Deserialize)]
#[serde(deny_unknown_fields)]
pub struct ProviderObservation {
    pub provider: ProviderKindV1,
    pub attempt: ExactRefV1,
    pub state: String,
    pub payload_digest: String,
    #[serde(default, skip_serializing_if = "Option::is_none")]
    pub checkpoint_artifact: Option<CheckpointArtifactObservationV1>,
    #[serde(default, skip_serializing_if = "Option::is_none")]
    pub workspace_commit: Option<WorkspaceCommitObservationV1>,
    pub observed_unix_nano: i64,
    pub digest: String,
}

#[derive(Clone, Debug, Eq, PartialEq, Serialize, Deserialize)]
#[serde(deny_unknown_fields)]
pub struct WorkspaceCommitObservationV1 {
    pub contract_version: String,
    pub change_set: ExactRefV1,
    pub view: ExactRefV1,
    pub base_revision: String,
    pub committed_revision: String,
    pub state: String,
    pub recorded_unix_nano: i64,
    pub expires_unix_nano: i64,
}

impl WorkspaceCommitObservationV1 {
    pub fn validate(&self, request: &DispatchRequestV1, now: i64) -> Result<()> {
        let crate::contract::ProviderPayloadV1::WorkspaceCommit(payload) = &request.payload else {
            return Err(ClosedError::new(
                ClosedReason::BindingDrift,
                "workspace commit observation reached another Provider payload",
            ));
        };
        let expected_expires = request
            .requested_not_after_unix_nano
            .min(request.sandbox_attempt.expires_unix_nano)
            .min(request.execution_binding.expires_unix_nano)
            .min(request.runtime_enforcement.expires_unix_nano)
            .min(payload.change_set.expires_unix_nano)
            .min(payload.view.expires_unix_nano);
        if self.contract_version != "praxis.sandbox/workspace-commit-observation/v1"
            || self.change_set != payload.change_set
            || self.view != payload.view
            || self.base_revision != payload.base_revision
            || !valid_checkpoint_digest(&self.committed_revision)
            || !matches!(
                self.state.as_str(),
                "prepared" | "committed" | "not_applied" | "indeterminate"
            )
            || self.recorded_unix_nano <= 0
            || self.recorded_unix_nano > now
            || self.expires_unix_nano != expected_expires
            || now >= self.expires_unix_nano
        {
            return Err(ClosedError::new(
                ClosedReason::InvalidContract,
                "workspace commit observation is incomplete or stale",
            ));
        }
        let valid_state = match (request.effect_kind.as_str(), request.phase) {
            ("praxis.sandbox/workspace-commit", EnforcementPhaseV1::Prepare) => {
                self.state == "prepared"
            }
            ("praxis.sandbox/workspace-commit", EnforcementPhaseV1::Execute) => {
                self.state == "committed"
            }
            ("praxis.sandbox/inspect", _) => {
                matches!(
                    self.state.as_str(),
                    "committed" | "not_applied" | "indeterminate"
                ) && payload.inspection_target.as_ref().is_some_and(|target| {
                    target.original_effect_kind == "praxis.sandbox/workspace-commit"
                })
            }
            _ => false,
        };
        if !valid_state {
            return Err(ClosedError::new(
                ClosedReason::BindingDrift,
                "workspace commit observation phase or state drifted",
            ));
        }
        Ok(())
    }
}

#[derive(Clone, Debug, Eq, PartialEq, Serialize, Deserialize)]
#[serde(deny_unknown_fields)]
pub struct CheckpointArtifactObservationV1 {
    pub contract_version: String,
    pub artifact_id: String,
    pub subject_digest: String,
    pub content_digest: String,
    pub content_length: u64,
    pub state: String,
    pub checkpoint_phase: String,
    pub recorded_unix_nano: i64,
    pub expires_unix_nano: i64,
}

impl CheckpointArtifactObservationV1 {
    pub fn validate(&self, request: &DispatchRequestV1, now: i64) -> Result<()> {
        if self.contract_version != "praxis.sandbox/checkpoint-artifact-observation/v1"
            || self.artifact_id.trim().is_empty()
            || !valid_checkpoint_digest(&self.subject_digest)
            || !valid_checkpoint_digest(&self.content_digest)
            || self.content_length == 0
            || !matches!(self.state.as_str(), "prepared" | "committed" | "aborted")
            || !matches!(
                self.checkpoint_phase.as_str(),
                "checkpoint_prepare" | "checkpoint_commit" | "checkpoint_abort"
            )
            || self.recorded_unix_nano <= 0
            || self.recorded_unix_nano > now
            || self.expires_unix_nano <= now
            || self.expires_unix_nano > request.requested_not_after_unix_nano
        {
            return Err(ClosedError::new(
                ClosedReason::InvalidContract,
                "checkpoint artifact observation is incomplete or stale",
            ));
        }
        let expected_state = match self.checkpoint_phase.as_str() {
            "checkpoint_prepare" => "prepared",
            "checkpoint_commit" => "committed",
            "checkpoint_abort" => "aborted",
            _ => unreachable!(),
        };
        if self.state != expected_state || !self.artifact_id.ends_with(&self.subject_digest[7..]) {
            return Err(ClosedError::new(
                ClosedReason::BindingDrift,
                "checkpoint artifact observation identity drifted",
            ));
        }
        Ok(())
    }
}

#[derive(Clone, Debug, Eq, PartialEq, Serialize, Deserialize)]
#[serde(deny_unknown_fields)]
pub struct ProviderReceipt {
    pub provider: ProviderKindV1,
    pub attempt: ExactRefV1,
    pub phase: String,
    pub observation_digest: String,
    pub recorded_unix_nano: i64,
    pub expires_unix_nano: i64,
    pub digest: String,
}

#[derive(Clone, Debug, Eq, PartialEq, Serialize, Deserialize)]
#[serde(deny_unknown_fields)]
pub struct ProviderResult {
    pub attempt: ExactRefV1,
    pub observation: ProviderObservation,
    pub receipt: ProviderReceipt,
}

impl ProviderResult {
    pub fn validate(&self, request: &DispatchRequestV1, now: i64) -> Result<()> {
        let mut expected_attempt = self.attempt.clone();
        expected_attempt.digest.clear();
        let expected_attempt_digest = canonical_digest("ProviderAttemptRefV1", &expected_attempt)?;
        let expected_expires = request
            .requested_not_after_unix_nano
            .min(request.sandbox_attempt.expires_unix_nano)
            .min(request.execution_binding.expires_unix_nano)
            .min(request.runtime_enforcement.expires_unix_nano);
        if now <= 0
            || self.attempt.id
                != format!(
                    "{}/{}/{}",
                    provider_name(request.payload.kind()),
                    request.tenant_id,
                    request.attempt_id
                )
            || self.attempt.revision != phase_revision(request.phase)
            || self.attempt.digest != expected_attempt_digest
            || self.attempt.expires_unix_nano != expected_expires
            || now >= self.attempt.expires_unix_nano
            || self.observation.provider != request.payload.kind()
            || self.observation.attempt != self.attempt
            || self.observation.state.trim().is_empty()
            || self.observation.payload_digest != request.payload_digest
            || self.observation.observed_unix_nano <= 0
            || self.observation.observed_unix_nano > now
            || self.receipt.provider != request.payload.kind()
            || self.receipt.attempt != self.attempt
            || self.receipt.phase != phase_name(request.phase)
            || self.receipt.observation_digest != self.observation.digest
            || self.receipt.recorded_unix_nano <= 0
            || self.receipt.recorded_unix_nano > now
            || self.receipt.expires_unix_nano != expected_expires
        {
            return Err(ClosedError::new(
                ClosedReason::InvalidContract,
                "provider result coordinates are invalid",
            ));
        }
        if request.effect_kind == crate::checkpoint::CHECKPOINT_EFFECT_KIND_V1
            && request.phase == EnforcementPhaseV1::Execute
        {
            self.observation
                .checkpoint_artifact
                .as_ref()
                .ok_or_else(|| {
                    ClosedError::new(
                        ClosedReason::InvalidContract,
                        "checkpoint Provider result lacks artifact observation",
                    )
                })?
                .validate(request, now)?;
        } else if self.observation.checkpoint_artifact.is_some() {
            return Err(ClosedError::new(
                ClosedReason::InvalidContract,
                "non-checkpoint Provider result carries artifact observation",
            ));
        }
        if request.payload.kind() == ProviderKindV1::WorkspaceCommit {
            self.observation
                .workspace_commit
                .as_ref()
                .ok_or_else(|| {
                    ClosedError::new(
                        ClosedReason::InvalidContract,
                        "workspace commit Provider result lacks exact commit observation",
                    )
                })?
                .validate(request, now)?;
        } else if self.observation.workspace_commit.is_some() {
            return Err(ClosedError::new(
                ClosedReason::InvalidContract,
                "non-workspace Provider result carries a workspace commit observation",
            ));
        }
        let mut observation = self.observation.clone();
        observation.digest.clear();
        let observation_digest = canonical_digest("ProviderObservationV1", &observation)?;
        let mut receipt = self.receipt.clone();
        receipt.digest.clear();
        let receipt_digest = canonical_digest("ProviderReceiptV1", &receipt)?;
        if self.observation.digest != observation_digest || self.receipt.digest != receipt_digest {
            return Err(ClosedError::new(
                ClosedReason::InvalidDigest,
                "provider result digest drifted",
            ));
        }
        Ok(())
    }
}

impl ProviderObservation {
    pub fn seal(mut self) -> Result<Self> {
        self.digest.clear();
        self.digest = canonical_digest("ProviderObservationV1", &self)?;
        Ok(self)
    }
}

impl ProviderResult {
    pub fn with_checkpoint_artifact(
        mut self,
        artifact: CheckpointArtifactObservationV1,
    ) -> Result<Self> {
        self.observation.checkpoint_artifact = Some(artifact);
        self.observation = self.observation.seal()?;
        self.receipt.observation_digest = self.observation.digest.clone();
        self.receipt = self.receipt.seal()?;
        Ok(self)
    }

    pub fn with_workspace_commit(
        mut self,
        mut observation: WorkspaceCommitObservationV1,
    ) -> Result<Self> {
        // Bind the nested observation to the same actual-point clock as its
        // enclosing Provider observation. Historical journal validation then
        // remains valid without consulting a newer wall clock.
        observation.recorded_unix_nano = self.observation.observed_unix_nano;
        self.observation.workspace_commit = Some(observation);
        self.observation = self.observation.seal()?;
        self.receipt.observation_digest = self.observation.digest.clone();
        self.receipt = self.receipt.seal()?;
        Ok(self)
    }
}

impl ProviderReceipt {
    pub fn seal(mut self) -> Result<Self> {
        self.digest.clear();
        self.digest = canonical_digest("ProviderReceiptV1", &self)?;
        Ok(self)
    }
}

pub fn provider_result(request: &DispatchRequestV1, state: &str) -> Result<ProviderResult> {
    let now = now_unix_nano();
    let expires = request
        .requested_not_after_unix_nano
        .min(request.sandbox_attempt.expires_unix_nano)
        .min(request.execution_binding.expires_unix_nano)
        .min(request.runtime_enforcement.expires_unix_nano);
    let revision = phase_revision(request.phase);
    let mut attempt = ExactRefV1 {
        id: format!(
            "{}/{}/{}",
            provider_name(request.payload.kind()),
            request.tenant_id,
            request.attempt_id
        ),
        revision,
        digest: String::new(),
        expires_unix_nano: expires,
    };
    attempt.digest = canonical_digest("ProviderAttemptRefV1", &attempt)?;
    let observation = ProviderObservation {
        provider: request.payload.kind(),
        attempt: attempt.clone(),
        state: state.to_owned(),
        payload_digest: request.payload_digest.clone(),
        checkpoint_artifact: None,
        workspace_commit: None,
        observed_unix_nano: now,
        digest: String::new(),
    }
    .seal()?;
    let receipt = ProviderReceipt {
        provider: request.payload.kind(),
        attempt: attempt.clone(),
        phase: phase_name(request.phase).to_owned(),
        observation_digest: observation.digest.clone(),
        recorded_unix_nano: now,
        expires_unix_nano: expires,
        digest: String::new(),
    }
    .seal()?;
    Ok(ProviderResult {
        attempt,
        observation,
        receipt,
    })
}

fn valid_checkpoint_digest(value: &str) -> bool {
    value.len() == 71
        && value.starts_with("sha256:")
        && value[7..].bytes().all(|byte| byte.is_ascii_hexdigit())
}

const fn phase_revision(phase: EnforcementPhaseV1) -> u64 {
    match phase {
        EnforcementPhaseV1::Prepare => 1,
        EnforcementPhaseV1::Execute => 2,
    }
}

const fn phase_name(phase: EnforcementPhaseV1) -> &'static str {
    match phase {
        EnforcementPhaseV1::Prepare => "prepare",
        EnforcementPhaseV1::Execute => "execute",
    }
}

const fn provider_name(provider: ProviderKindV1) -> &'static str {
    match provider {
        ProviderKindV1::HostWorkspace => "host",
        ProviderKindV1::QemuMicrovm => "microvm",
        ProviderKindV1::ContainerdOci => "containerd",
        ProviderKindV1::WasmtimeComponent => "wasmtime",
        ProviderKindV1::RemoteSandbox => "remote",
        ProviderKindV1::WorkspaceCommit => "workspace-commit",
    }
}

#[async_trait]
pub trait Provider: Send + Sync {
    fn kind(&self) -> ProviderKindV1;

    async fn prepare(&self, request: &DispatchRequestV1) -> Result<ProviderResult>;
    async fn execute_prepared(&self, request: &DispatchRequestV1) -> Result<ProviderResult>;
    async fn inspect(&self, request: &DispatchRequestV1) -> Result<ProviderResult>;
    async fn fence(&self, request: &DispatchRequestV1) -> Result<ProviderResult>;
    async fn release(&self, request: &DispatchRequestV1) -> Result<ProviderResult>;
    async fn cleanup(&self, request: &DispatchRequestV1) -> Result<ProviderResult>;
    async fn inspect_cleanup(&self, request: &DispatchRequestV1) -> Result<ProviderResult>;

    async fn checkpoint_source(&self, _request: &DispatchRequestV1) -> Result<CheckpointSource> {
        Err(ClosedError::new(
            ClosedReason::Unsupported,
            "provider does not implement a checkpoint source",
        ))
    }
}

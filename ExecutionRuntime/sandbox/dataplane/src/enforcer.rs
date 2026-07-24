use std::sync::Arc;

use async_trait::async_trait;

use crate::checkpoint::{CHECKPOINT_EFFECT_KIND_V1, CheckpointStore};
use crate::contract::{
    CurrentAuthorizationV1, DispatchRequestV1, EnforcementPhaseV1, now_unix_nano,
};
use crate::error::{ClosedError, ClosedReason, Result};
use crate::journal::AttemptJournal;
use crate::provider::{Provider, ProviderResult};

#[async_trait]
pub trait CurrentFactsReader: Send + Sync {
    async fn inspect_current(&self, request: &DispatchRequestV1) -> Result<CurrentAuthorizationV1>;
}

pub struct DataPlaneEnforcer<R: CurrentFactsReader> {
    reader: Arc<R>,
    journal: Arc<AttemptJournal>,
    checkpoint: Option<Arc<CheckpointStore>>,
}

impl<R: CurrentFactsReader> DataPlaneEnforcer<R> {
    #[must_use]
    pub fn new(reader: Arc<R>, journal: Arc<AttemptJournal>) -> Self {
        Self {
            reader,
            journal,
            checkpoint: None,
        }
    }

    #[must_use]
    pub fn with_checkpoint_store(mut self, checkpoint: Arc<CheckpointStore>) -> Self {
        self.checkpoint = Some(checkpoint);
        self
    }

    pub async fn dispatch(
        &self,
        provider: &dyn Provider,
        request: &DispatchRequestV1,
    ) -> Result<ProviderResult> {
        request.validate_current(now_unix_nano())?;
        if provider.kind() != request.payload.kind() {
            return Err(ClosedError::new(
                ClosedReason::BindingDrift,
                "provider kind does not match the sealed payload",
            ));
        }
        let current = self.reader.inspect_current(request).await?;
        current.validate_against(request, now_unix_nano())?;

        self.journal.begin(request).await?;
        let result = match request.phase {
            EnforcementPhaseV1::Prepare if request.effect_kind == CHECKPOINT_EFFECT_KIND_V1 => {
                self.checkpoint_store()?.prepare(provider, request).await
            }
            EnforcementPhaseV1::Prepare => provider.prepare(request).await,
            EnforcementPhaseV1::Execute => match request.effect_kind.as_str() {
                "praxis.sandbox/backend-discovery" | "praxis.sandbox/inspect" => {
                    provider.inspect(request).await
                }
                "praxis.sandbox/allocate"
                | "praxis.sandbox/activate"
                | "praxis.sandbox/open"
                | "praxis.sandbox/workspace-commit" => provider.execute_prepared(request).await,
                "praxis.sandbox/cancel" | "praxis.sandbox/close" | "praxis.sandbox/fence" => {
                    provider.fence(request).await
                }
                "praxis.sandbox/release" => provider.release(request).await,
                "praxis.sandbox/cleanup" => provider.cleanup(request).await,
                CHECKPOINT_EFFECT_KIND_V1 => {
                    self.checkpoint_store()?.execute(provider, request).await
                }
                _ => Err(ClosedError::new(
                    ClosedReason::Unsupported,
                    "sandbox effect kind is unsupported",
                )),
            },
        }?;
        self.journal.complete(request, &result).await?;
        Ok(result)
    }

    /// Inspect is the only recovery path after a dispatch reply is lost. It
    /// returns the exact durable result and cannot enter a Provider method.
    pub async fn inspect(&self, request: &DispatchRequestV1) -> Result<ProviderResult> {
        match self.journal.inspect(request).await {
            Ok(result) => Ok(result),
            Err(error) if request.effect_kind == CHECKPOINT_EFFECT_KIND_V1 => {
                match self.checkpoint_store()?.inspect(request).await {
                    Ok(result) => Ok(result),
                    Err(_) => Err(error),
                }
            }
            Err(error) => Err(error),
        }
    }

    fn checkpoint_store(&self) -> Result<&CheckpointStore> {
        self.checkpoint.as_deref().ok_or_else(|| {
            ClosedError::new(
                ClosedReason::Unsupported,
                "checkpoint Provider store is not configured",
            )
        })
    }
}

use async_trait::async_trait;
use serde::{Deserialize, Serialize};

use crate::contract::{DispatchRequestV1, ProviderKindV1, ProviderPayloadV1, now_unix_nano};
use crate::error::{ClosedError, ClosedReason, Result};
use crate::provider::{Provider, ProviderResult};

#[derive(Clone, Copy, Debug, Eq, PartialEq, Serialize, Deserialize)]
#[serde(rename_all = "snake_case")]
pub enum RemoteOperationV1 {
    Prepare,
    ExecutePrepared,
    Inspect,
    Fence,
    Release,
    Cleanup,
    InspectCleanup,
}

/// Protocol-neutral remote connector. Endpoint resolution and credential
/// material remain inside the trusted implementation; Sandbox objects carry
/// only exact binding coordinates.
#[async_trait]
pub trait RemoteTransport: Send + Sync {
    async fn invoke(
        &self,
        operation: RemoteOperationV1,
        request: &DispatchRequestV1,
    ) -> Result<ProviderResult>;
}

pub struct RemoteProvider<T> {
    transport: T,
}

impl<T> RemoteProvider<T> {
    pub const fn new(transport: T) -> Self {
        Self { transport }
    }
}

impl<T: RemoteTransport> RemoteProvider<T> {
    fn validate_current(request: &DispatchRequestV1) -> Result<()> {
        let ProviderPayloadV1::RemoteSandbox(payload) = &request.payload else {
            return Err(ClosedError::new(
                ClosedReason::BindingDrift,
                "non-remote payload reached Remote Provider",
            ));
        };
        let now = now_unix_nano();
        payload
            .credential
            .validate_current("remote credential", now)?;
        if payload.credential.expires_unix_nano > request.requested_not_after_unix_nano {
            return Err(ClosedError::new(
                ClosedReason::BindingDrift,
                "remote credential extends the governed request TTL",
            ));
        }
        Ok(())
    }

    async fn invoke_or_inspect(
        &self,
        operation: RemoteOperationV1,
        request: &DispatchRequestV1,
    ) -> Result<ProviderResult> {
        Self::validate_current(request)?;
        let result = match self.transport.invoke(operation, request).await {
            Ok(result) => result,
            Err(_) => {
                // An ambiguous Provider reply never grants redispatch. Only
                // exact Inspect of the original Attempt is permitted.
                self.transport
                    .invoke(RemoteOperationV1::Inspect, request)
                    .await?
            }
        };
        result.validate(request, now_unix_nano())?;
        Ok(result)
    }
}

#[async_trait]
impl<T: RemoteTransport> Provider for RemoteProvider<T> {
    fn kind(&self) -> ProviderKindV1 {
        ProviderKindV1::RemoteSandbox
    }

    async fn prepare(&self, request: &DispatchRequestV1) -> Result<ProviderResult> {
        self.invoke_or_inspect(RemoteOperationV1::Prepare, request)
            .await
    }

    async fn execute_prepared(&self, request: &DispatchRequestV1) -> Result<ProviderResult> {
        self.invoke_or_inspect(RemoteOperationV1::ExecutePrepared, request)
            .await
    }

    async fn inspect(&self, request: &DispatchRequestV1) -> Result<ProviderResult> {
        Self::validate_current(request)?;
        let operation = if request
            .payload
            .inspection_target()
            .is_some_and(|target| target.original_effect_kind == "praxis.sandbox/cleanup")
        {
            RemoteOperationV1::InspectCleanup
        } else {
            RemoteOperationV1::Inspect
        };
        let result = self.transport.invoke(operation, request).await?;
        result.validate(request, now_unix_nano())?;
        Ok(result)
    }

    async fn fence(&self, request: &DispatchRequestV1) -> Result<ProviderResult> {
        self.invoke_or_inspect(RemoteOperationV1::Fence, request)
            .await
    }

    async fn release(&self, request: &DispatchRequestV1) -> Result<ProviderResult> {
        self.invoke_or_inspect(RemoteOperationV1::Release, request)
            .await
    }

    async fn cleanup(&self, request: &DispatchRequestV1) -> Result<ProviderResult> {
        self.invoke_or_inspect(RemoteOperationV1::Cleanup, request)
            .await
    }

    async fn inspect_cleanup(&self, request: &DispatchRequestV1) -> Result<ProviderResult> {
        Self::validate_current(request)?;
        let result = self
            .transport
            .invoke(RemoteOperationV1::InspectCleanup, request)
            .await?;
        result.validate(request, now_unix_nano())?;
        Ok(result)
    }
}

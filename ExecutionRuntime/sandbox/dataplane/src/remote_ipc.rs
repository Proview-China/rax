use std::path::PathBuf;

use async_trait::async_trait;
use serde::{Deserialize, Serialize};

use crate::contract::{DispatchRequestV1, canonical_digest};
use crate::error::{ClosedError, ClosedReason, Result};
use crate::ipc::{connect, read_frame, validate_peer, write_frame};
use crate::provider::ProviderResult;
use crate::remote::{RemoteOperationV1, RemoteTransport};

pub const REMOTE_CONNECTOR_IPC_VERSION_V1: &str = "praxis.sandbox/remote-connector-ipc/v1";

#[derive(Clone, Debug, Eq, PartialEq, Serialize, Deserialize)]
#[serde(deny_unknown_fields)]
pub struct RemoteConnectorRequestV1 {
    pub contract_version: String,
    pub operation: RemoteOperationV1,
    pub request: DispatchRequestV1,
    pub digest: String,
}

impl RemoteConnectorRequestV1 {
    pub fn seal(operation: RemoteOperationV1, request: DispatchRequestV1) -> Result<Self> {
        let mut value = Self {
            contract_version: REMOTE_CONNECTOR_IPC_VERSION_V1.to_owned(),
            operation,
            request,
            digest: String::new(),
        };
        value.digest = canonical_digest("RemoteConnectorRequestV1", &value)?;
        value.validate()?;
        Ok(value)
    }

    pub fn validate(&self) -> Result<()> {
        let mut copy = self.clone();
        copy.digest.clear();
        if self.contract_version != REMOTE_CONNECTOR_IPC_VERSION_V1
            || self.request.validate_shape().is_err()
            || self.digest != canonical_digest("RemoteConnectorRequestV1", &copy)?
        {
            return Err(ClosedError::new(
                ClosedReason::InvalidContract,
                "remote connector request is invalid",
            ));
        }
        Ok(())
    }
}

#[derive(Clone, Debug, Eq, PartialEq, Serialize, Deserialize)]
#[serde(deny_unknown_fields)]
pub struct RemoteConnectorResponseV1 {
    pub contract_version: String,
    pub request_digest: String,
    #[serde(default, skip_serializing_if = "Option::is_none")]
    pub result: Option<ProviderResult>,
    #[serde(default, skip_serializing_if = "Option::is_none")]
    pub error: Option<ClosedError>,
    pub digest: String,
}

impl RemoteConnectorResponseV1 {
    pub fn seal(
        request_digest: String,
        result: Option<ProviderResult>,
        error: Option<ClosedError>,
    ) -> Result<Self> {
        let mut value = Self {
            contract_version: REMOTE_CONNECTOR_IPC_VERSION_V1.to_owned(),
            request_digest,
            result,
            error,
            digest: String::new(),
        };
        value.digest = canonical_digest("RemoteConnectorResponseV1", &value)?;
        value.validate()?;
        Ok(value)
    }

    pub fn validate(&self) -> Result<()> {
        let mut copy = self.clone();
        copy.digest.clear();
        if self.contract_version != REMOTE_CONNECTOR_IPC_VERSION_V1
            || self.request_digest.trim().is_empty()
            || matches!(
                (&self.result, &self.error),
                (Some(_), Some(_)) | (None, None)
            )
            || self.digest != canonical_digest("RemoteConnectorResponseV1", &copy)?
        {
            return Err(ClosedError::new(
                ClosedReason::InvalidContract,
                "remote connector response is invalid",
            ));
        }
        Ok(())
    }
}

pub struct SocketRemoteTransport {
    path: PathBuf,
    allowed_uid: u32,
}

impl SocketRemoteTransport {
    pub fn new(path: PathBuf, allowed_uid: u32) -> Result<Self> {
        if path.as_os_str().is_empty() {
            return Err(ClosedError::new(
                ClosedReason::InvalidArgument,
                "remote connector socket is required",
            ));
        }
        Ok(Self { path, allowed_uid })
    }
}

#[async_trait]
impl RemoteTransport for SocketRemoteTransport {
    async fn invoke(
        &self,
        operation: RemoteOperationV1,
        request: &DispatchRequestV1,
    ) -> Result<ProviderResult> {
        let envelope = RemoteConnectorRequestV1::seal(operation, request.clone())?;
        let mut stream = connect(&self.path).await?;
        validate_peer(&stream, self.allowed_uid)?;
        write_frame(&mut stream, &envelope).await?;
        let response: RemoteConnectorResponseV1 = read_frame(&mut stream).await?;
        response.validate()?;
        if response.request_digest != envelope.digest {
            return Err(ClosedError::new(
                ClosedReason::BindingDrift,
                "remote connector response crosses request",
            ));
        }
        match (response.result, response.error) {
            (Some(result), None) => Ok(result),
            (None, Some(error)) => Err(error),
            _ => Err(ClosedError::new(
                ClosedReason::InvalidContract,
                "remote connector response presence is invalid",
            )),
        }
    }
}

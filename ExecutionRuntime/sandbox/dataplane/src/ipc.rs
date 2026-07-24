use std::path::Path;

use nix::sys::socket::{getsockopt, sockopt::PeerCredentials};
use serde::Serialize;
use serde::de::DeserializeOwned;
use tokio::io::{AsyncRead, AsyncReadExt, AsyncWrite, AsyncWriteExt};
use tokio::net::UnixStream;

use async_trait::async_trait;

use crate::contract::{
    CurrentAuthorizationV1, CurrentReadResponseV1, DispatchRequestV1, MAX_FRAME_BYTES,
};
use crate::enforcer::CurrentFactsReader;
use crate::error::{ClosedError, ClosedReason, Result};

pub async fn read_frame<T: DeserializeOwned>(stream: &mut (impl AsyncRead + Unpin)) -> Result<T> {
    let length = stream.read_u32().await.map_err(ClosedError::internal)? as usize;
    if length == 0 || length > MAX_FRAME_BYTES {
        return Err(ClosedError::new(
            ClosedReason::FrameTooLarge,
            "IPC frame length is outside the closed bounds",
        ));
    }
    let mut bytes = vec![0_u8; length];
    stream
        .read_exact(&mut bytes)
        .await
        .map_err(ClosedError::internal)?;
    let mut deserializer = serde_json::Deserializer::from_slice(&bytes);
    let value = T::deserialize(&mut deserializer).map_err(|_| {
        ClosedError::new(
            ClosedReason::InvalidContract,
            "IPC JSON contract is invalid",
        )
    })?;
    deserializer.end().map_err(|_| {
        ClosedError::new(ClosedReason::InvalidContract, "IPC frame has trailing data")
    })?;
    Ok(value)
}

pub async fn write_frame<T: Serialize>(
    stream: &mut (impl AsyncWrite + Unpin),
    value: &T,
) -> Result<()> {
    let bytes = serde_json::to_vec(value).map_err(ClosedError::internal)?;
    let length = u32::try_from(bytes.len()).map_err(ClosedError::internal)?;
    if bytes.is_empty() || bytes.len() > MAX_FRAME_BYTES {
        return Err(ClosedError::new(
            ClosedReason::FrameTooLarge,
            "IPC response length is outside the closed bounds",
        ));
    }
    stream
        .write_u32(length)
        .await
        .map_err(ClosedError::internal)?;
    stream
        .write_all(&bytes)
        .await
        .map_err(ClosedError::internal)?;
    stream.flush().await.map_err(ClosedError::internal)
}

pub async fn connect(path: impl AsRef<Path>) -> Result<UnixStream> {
    UnixStream::connect(path)
        .await
        .map_err(|_| ClosedError::new(ClosedReason::CurrentUnavailable, "IPC peer is unavailable"))
}

pub fn validate_peer(stream: &UnixStream, allowed_uid: u32) -> Result<()> {
    let credentials = getsockopt(stream, PeerCredentials).map_err(ClosedError::internal)?;
    if credentials.uid() != allowed_uid {
        return Err(ClosedError::new(
            ClosedReason::UnauthorizedPeer,
            "IPC peer UID is not authorized",
        ));
    }
    Ok(())
}

pub struct SocketCurrentFactsReader {
    path: std::path::PathBuf,
    allowed_uid: u32,
}

impl SocketCurrentFactsReader {
    #[must_use]
    pub fn new(path: impl Into<std::path::PathBuf>, allowed_uid: u32) -> Self {
        Self {
            path: path.into(),
            allowed_uid,
        }
    }
}

#[async_trait]
impl CurrentFactsReader for SocketCurrentFactsReader {
    async fn inspect_current(&self, request: &DispatchRequestV1) -> Result<CurrentAuthorizationV1> {
        let mut stream = connect(&self.path).await?;
        validate_peer(&stream, self.allowed_uid)?;
        write_frame(&mut stream, request).await?;
        let response: CurrentReadResponseV1 = read_frame(&mut stream).await?;
        match (response.authorization, response.error) {
            (Some(authorization), None) => Ok(authorization),
            (None, Some(error)) => Err(error),
            _ => Err(ClosedError::new(
                ClosedReason::InvalidContract,
                "current reader response presence is invalid",
            )),
        }
    }
}

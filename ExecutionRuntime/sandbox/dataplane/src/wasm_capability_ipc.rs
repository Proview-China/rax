use std::io::{Read, Write};
use std::os::unix::net::UnixStream;
use std::path::PathBuf;
use std::time::Duration;

use nix::sys::socket::{getsockopt, sockopt::PeerCredentials};
use serde::{Deserialize, Serialize};

use crate::contract::{MAX_FRAME_BYTES, WasmCapabilityBindingV1, canonical_digest};
use crate::error::{ClosedError, ClosedReason, Result};
use crate::wasm::WasmCapabilityHost;

pub const WASM_CAPABILITY_IPC_VERSION_V1: &str = "praxis.sandbox/wasm-capability-ipc/v1";

#[derive(Clone, Debug, Eq, PartialEq, Serialize, Deserialize)]
#[serde(deny_unknown_fields)]
pub struct WasmCapabilityInvokeRequestV1 {
    pub contract_version: String,
    pub binding: WasmCapabilityBindingV1,
    pub request: String,
    pub digest: String,
}

impl WasmCapabilityInvokeRequestV1 {
    pub fn seal(binding: WasmCapabilityBindingV1, request: String) -> Result<Self> {
        let mut value = Self {
            contract_version: WASM_CAPABILITY_IPC_VERSION_V1.to_owned(),
            binding,
            request,
            digest: String::new(),
        };
        value.digest = canonical_digest("WasmCapabilityInvokeRequestV1", &value)?;
        Ok(value)
    }

    pub fn validate(&self) -> Result<()> {
        let mut copy = self.clone();
        copy.digest.clear();
        if self.contract_version != WASM_CAPABILITY_IPC_VERSION_V1
            || self.request.len() as u64 > self.binding.max_request_bytes
            || self.digest != canonical_digest("WasmCapabilityInvokeRequestV1", &copy)?
        {
            return Err(ClosedError::new(
                ClosedReason::InvalidContract,
                "WASM capability IPC request is invalid",
            ));
        }
        Ok(())
    }
}

#[derive(Clone, Debug, Eq, PartialEq, Serialize, Deserialize)]
#[serde(deny_unknown_fields)]
pub struct WasmCapabilityInvokeResponseV1 {
    pub contract_version: String,
    pub request_digest: String,
    #[serde(default, skip_serializing_if = "Option::is_none")]
    pub result: Option<String>,
    #[serde(default, skip_serializing_if = "Option::is_none")]
    pub error: Option<ClosedError>,
    pub digest: String,
}

impl WasmCapabilityInvokeResponseV1 {
    pub fn seal(
        request_digest: String,
        result: Option<String>,
        error: Option<ClosedError>,
    ) -> Result<Self> {
        let mut value = Self {
            contract_version: WASM_CAPABILITY_IPC_VERSION_V1.to_owned(),
            request_digest,
            result,
            error,
            digest: String::new(),
        };
        value.digest = canonical_digest("WasmCapabilityInvokeResponseV1", &value)?;
        value.validate()?;
        Ok(value)
    }

    pub fn validate(&self) -> Result<()> {
        let mut copy = self.clone();
        copy.digest.clear();
        if self.contract_version != WASM_CAPABILITY_IPC_VERSION_V1
            || self.request_digest.trim().is_empty()
            || matches!(
                (&self.result, &self.error),
                (Some(_), Some(_)) | (None, None)
            )
            || self.digest != canonical_digest("WasmCapabilityInvokeResponseV1", &copy)?
        {
            return Err(ClosedError::new(
                ClosedReason::InvalidContract,
                "WASM capability IPC response is invalid",
            ));
        }
        Ok(())
    }
}

pub struct SocketWasmCapabilityHost {
    path: PathBuf,
    allowed_uid: u32,
    timeout: Duration,
}

impl SocketWasmCapabilityHost {
    pub fn new(path: PathBuf, allowed_uid: u32, timeout: Duration) -> Result<Self> {
        if path.as_os_str().is_empty() || timeout.is_zero() || timeout > Duration::from_secs(30) {
            return Err(ClosedError::new(
                ClosedReason::InvalidArgument,
                "WASM capability IPC configuration is invalid",
            ));
        }
        Ok(Self {
            path,
            allowed_uid,
            timeout,
        })
    }
}

impl WasmCapabilityHost for SocketWasmCapabilityHost {
    fn invoke(&self, binding: &WasmCapabilityBindingV1, request: &str) -> Result<String> {
        let envelope = WasmCapabilityInvokeRequestV1::seal(binding.clone(), request.to_owned())?;
        envelope.validate()?;
        let mut stream = UnixStream::connect(&self.path).map_err(|_| {
            ClosedError::new(
                ClosedReason::ProviderUnavailable,
                "WASM capability Gateway is unavailable",
            )
        })?;
        stream
            .set_read_timeout(Some(self.timeout))
            .map_err(ClosedError::internal)?;
        stream
            .set_write_timeout(Some(self.timeout))
            .map_err(ClosedError::internal)?;
        let credentials = getsockopt(&stream, PeerCredentials).map_err(ClosedError::internal)?;
        if credentials.uid() != self.allowed_uid {
            return Err(ClosedError::new(
                ClosedReason::UnauthorizedPeer,
                "WASM capability Gateway peer UID is not authorized",
            ));
        }
        write_sync_frame(&mut stream, &envelope)?;
        let response: WasmCapabilityInvokeResponseV1 = read_sync_frame(&mut stream)?;
        response.validate()?;
        if response.request_digest != envelope.digest {
            return Err(ClosedError::new(
                ClosedReason::BindingDrift,
                "WASM capability Gateway response crosses request",
            ));
        }
        match (response.result, response.error) {
            (Some(result), None) if result.len() as u64 <= binding.max_response_bytes => Ok(result),
            (Some(_), None) => Err(ClosedError::new(
                ClosedReason::ResourceLimit,
                "WASM capability Gateway response exceeds its sealed limit",
            )),
            (None, Some(error)) => Err(error),
            _ => Err(ClosedError::new(
                ClosedReason::InvalidContract,
                "WASM capability Gateway response presence is invalid",
            )),
        }
    }
}

fn write_sync_frame<T: Serialize>(stream: &mut UnixStream, value: &T) -> Result<()> {
    let payload = serde_json::to_vec(value).map_err(ClosedError::internal)?;
    if payload.is_empty() || payload.len() > MAX_FRAME_BYTES {
        return Err(ClosedError::new(
            ClosedReason::FrameTooLarge,
            "WASM capability IPC request is outside the closed bounds",
        ));
    }
    let length = u32::try_from(payload.len()).map_err(ClosedError::internal)?;
    stream
        .write_all(&length.to_be_bytes())
        .map_err(ClosedError::internal)?;
    stream.write_all(&payload).map_err(ClosedError::internal)?;
    stream.flush().map_err(ClosedError::internal)
}

fn read_sync_frame<T: for<'de> Deserialize<'de>>(stream: &mut UnixStream) -> Result<T> {
    let mut length = [0_u8; 4];
    stream
        .read_exact(&mut length)
        .map_err(ClosedError::internal)?;
    let length = u32::from_be_bytes(length) as usize;
    if length == 0 || length > MAX_FRAME_BYTES {
        return Err(ClosedError::new(
            ClosedReason::FrameTooLarge,
            "WASM capability IPC response is outside the closed bounds",
        ));
    }
    let mut payload = vec![0_u8; length];
    stream
        .read_exact(&mut payload)
        .map_err(ClosedError::internal)?;
    let mut deserializer = serde_json::Deserializer::from_slice(&payload);
    let value = T::deserialize(&mut deserializer).map_err(|_| {
        ClosedError::new(
            ClosedReason::InvalidContract,
            "WASM capability IPC JSON is invalid",
        )
    })?;
    deserializer.end().map_err(|_| {
        ClosedError::new(
            ClosedReason::InvalidContract,
            "WASM capability IPC has trailing data",
        )
    })?;
    Ok(value)
}

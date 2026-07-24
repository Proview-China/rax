use serde::{Deserialize, Serialize};
use thiserror::Error;

pub type Result<T> = std::result::Result<T, ClosedError>;

#[derive(Clone, Copy, Debug, Eq, PartialEq, Serialize, Deserialize)]
#[serde(rename_all = "snake_case")]
pub enum ClosedReason {
    InvalidArgument,
    InvalidContract,
    InvalidDigest,
    UnauthorizedPeer,
    FrameTooLarge,
    BindingDrift,
    CurrentUnavailable,
    CurrentExpired,
    Conflict,
    NotFoundObservation,
    ProviderUnavailable,
    ProviderUnknown,
    ResourceLimit,
    Unsupported,
    Internal,
}

#[derive(Clone, Debug, Eq, Error, PartialEq, Serialize, Deserialize)]
#[error("{reason:?}: {message}")]
pub struct ClosedError {
    pub reason: ClosedReason,
    pub message: String,
}

impl ClosedError {
    #[must_use]
    pub fn new(reason: ClosedReason, message: impl Into<String>) -> Self {
        Self {
            reason,
            message: message.into(),
        }
    }

    #[must_use]
    pub fn internal(_error: impl std::fmt::Display) -> Self {
        Self::new(ClosedReason::Internal, "data plane internal failure")
    }
}

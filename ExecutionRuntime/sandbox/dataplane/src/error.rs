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

#[derive(Clone, Copy, Debug, Eq, PartialEq, Serialize, Deserialize)]
#[serde(rename_all = "snake_case")]
pub enum EffectBoundary {
    EffectNotStarted,
    EffectStartedUnknown,
}

#[derive(Clone, Debug, Eq, Error, PartialEq, Serialize, Deserialize)]
#[error("{reason:?}: {message}")]
pub struct ClosedError {
    pub reason: ClosedReason,
    pub message: String,
    #[serde(default, skip_serializing_if = "Option::is_none")]
    pub effect_boundary: Option<EffectBoundary>,
    #[serde(default, skip_serializing_if = "Option::is_none")]
    pub crossed_actual_point: Option<bool>,
    #[serde(default, skip_serializing_if = "Option::is_none")]
    pub physical_read_count: Option<u64>,
}

impl ClosedError {
    #[must_use]
    pub fn new(reason: ClosedReason, message: impl Into<String>) -> Self {
        Self {
            reason,
            message: message.into(),
            effect_boundary: None,
            crossed_actual_point: None,
            physical_read_count: None,
        }
    }

    #[must_use]
    pub fn with_effect_boundary(mut self, boundary: EffectBoundary) -> Self {
        self.effect_boundary = Some(boundary);
        self
    }

    #[must_use]
    pub fn with_actual_point_evidence(mut self, crossed: bool, physical_read_count: u64) -> Self {
        self.crossed_actual_point = Some(crossed);
        self.physical_read_count = Some(physical_read_count);
        self
    }

    #[must_use]
    pub fn internal(_error: impl std::fmt::Display) -> Self {
        Self::new(ClosedReason::Internal, "data plane internal failure")
    }
}

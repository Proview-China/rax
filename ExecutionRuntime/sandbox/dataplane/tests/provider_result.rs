mod common;

use praxis_sandbox_dataplane::contract::{DispatchResponseV1, EnforcementPhaseV1};
use praxis_sandbox_dataplane::provider::provider_result;

#[test]
fn self_consistent_provider_objects_cannot_forge_attempt_digest() {
    let request = common::request(EnforcementPhaseV1::Prepare);
    let mut result = provider_result(&request, "prepared")
        .unwrap_or_else(|error| panic!("provider result: {error}"));
    result.attempt.digest = common::digest("forged-attempt");
    result.observation.attempt = result.attempt.clone();
    result.observation = result
        .observation
        .seal()
        .unwrap_or_else(|error| panic!("seal observation: {error}"));
    result.receipt.attempt = result.attempt.clone();
    result.receipt.observation_digest = result.observation.digest.clone();
    result.receipt = result
        .receipt
        .seal()
        .unwrap_or_else(|error| panic!("seal receipt: {error}"));
    let error = common::must_error(DispatchResponseV1::success(&request, &result));
    assert_eq!(
        error.reason,
        praxis_sandbox_dataplane::error::ClosedReason::InvalidContract
    );
}

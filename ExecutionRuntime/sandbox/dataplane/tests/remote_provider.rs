mod common;

use std::sync::Arc;
use std::sync::atomic::{AtomicBool, AtomicUsize, Ordering};

use async_trait::async_trait;
use praxis_sandbox_dataplane::contract::{
    EnforcementPhaseV1, ExactRefV1, ProviderPayloadV1, RemotePayloadV1, now_unix_nano,
};
use praxis_sandbox_dataplane::error::{ClosedError, ClosedReason, Result};
use praxis_sandbox_dataplane::provider::{Provider, ProviderResult, provider_result};
use praxis_sandbox_dataplane::remote::{RemoteOperationV1, RemoteProvider, RemoteTransport};
use tokio::sync::Mutex;

struct ProbeState {
    calls: Mutex<Vec<RemoteOperationV1>>,
    fail_first: AtomicBool,
    executes: AtomicUsize,
    forge: AtomicBool,
}

impl ProbeState {
    fn new(fail_first: bool) -> Self {
        Self {
            calls: Mutex::new(Vec::new()),
            fail_first: AtomicBool::new(fail_first),
            executes: AtomicUsize::new(0),
            forge: AtomicBool::new(false),
        }
    }
}

#[derive(Clone)]
struct ProbeTransport(Arc<ProbeState>);

#[async_trait]
impl RemoteTransport for ProbeTransport {
    async fn invoke(
        &self,
        operation: RemoteOperationV1,
        request: &praxis_sandbox_dataplane::DispatchRequestV1,
    ) -> Result<ProviderResult> {
        self.0.calls.lock().await.push(operation);
        if operation == RemoteOperationV1::ExecutePrepared {
            self.0.executes.fetch_add(1, Ordering::SeqCst);
        }
        if self.0.fail_first.swap(false, Ordering::SeqCst) {
            return Err(ClosedError::new(
                ClosedReason::ProviderUnknown,
                "injected remote lost reply",
            ));
        }
        let mut result = provider_result(request, "remote-complete")?;
        if self.0.forge.load(Ordering::SeqCst) {
            result.receipt.digest = common::digest("forged-remote-receipt");
        }
        Ok(result)
    }
}

fn remote_request() -> praxis_sandbox_dataplane::DispatchRequestV1 {
    let expires = now_unix_nano() + 30_000_000_000;
    common::request_with_payload(
        EnforcementPhaseV1::Execute,
        ProviderPayloadV1::RemoteSandbox(RemotePayloadV1 {
            endpoint_binding_id: "remote-endpoint-1".to_owned(),
            endpoint_digest: common::digest("remote-endpoint-1"),
            workload_id: "workload-1".to_owned(),
            workload_digest: common::digest("workload-1"),
            credential: ExactRefV1 {
                id: "credential-1".to_owned(),
                revision: 1,
                digest: common::digest("credential-1"),
                expires_unix_nano: expires,
            },
            isolation_profile: "strong-tenant-isolation".to_owned(),
            inspection_target: None,
        }),
    )
}

#[tokio::test]
async fn lost_reply_only_inspects_original_remote_attempt() {
    let state = Arc::new(ProbeState::new(true));
    let provider = RemoteProvider::new(ProbeTransport(Arc::clone(&state)));
    let result = provider
        .execute_prepared(&remote_request())
        .await
        .unwrap_or_else(|error| panic!("remote recovery: {error}"));
    assert_eq!(result.observation.state, "remote-complete");
    assert_eq!(state.executes.load(Ordering::SeqCst), 1);
    assert_eq!(
        *state.calls.lock().await,
        vec![
            RemoteOperationV1::ExecutePrepared,
            RemoteOperationV1::Inspect
        ]
    );
}

#[tokio::test]
async fn expired_remote_credential_fails_before_transport() {
    let mut request = remote_request();
    let ProviderPayloadV1::RemoteSandbox(payload) = &mut request.payload else {
        unreachable!();
    };
    payload.credential.expires_unix_nano = now_unix_nano();
    request = request
        .seal()
        .unwrap_or_else(|error| panic!("expired request shape: {error}"));
    let state = Arc::new(ProbeState::new(false));
    let provider = RemoteProvider::new(ProbeTransport(Arc::clone(&state)));
    let error = common::must_error(provider.execute_prepared(&request).await);
    assert_eq!(error.reason, ClosedReason::CurrentExpired);
    assert!(state.calls.lock().await.is_empty());
}

#[tokio::test]
async fn remote_transport_cannot_forge_receipt_digest() {
    let state = Arc::new(ProbeState::new(false));
    state.forge.store(true, Ordering::SeqCst);
    let provider = RemoteProvider::new(ProbeTransport(state));
    let error = common::must_error(provider.execute_prepared(&remote_request()).await);
    assert_eq!(error.reason, ClosedReason::InvalidDigest);
}

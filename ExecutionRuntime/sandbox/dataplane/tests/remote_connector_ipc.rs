mod common;

use nix::unistd::Uid;
use praxis_sandbox_dataplane::contract::{
    EnforcementPhaseV1, ExactRefV1, ProviderPayloadV1, RemotePayloadV1, now_unix_nano,
};
use praxis_sandbox_dataplane::error::ClosedReason;
use praxis_sandbox_dataplane::ipc::{read_frame, write_frame};
use praxis_sandbox_dataplane::provider::provider_result;
use praxis_sandbox_dataplane::remote::{RemoteOperationV1, RemoteTransport};
use praxis_sandbox_dataplane::remote_ipc::{
    RemoteConnectorRequestV1, RemoteConnectorResponseV1, SocketRemoteTransport,
};
use tokio::net::UnixListener;

fn remote_request() -> praxis_sandbox_dataplane::DispatchRequestV1 {
    let expires = now_unix_nano() + 30_000_000_000;
    common::request_with_payload(
        EnforcementPhaseV1::Execute,
        ProviderPayloadV1::RemoteSandbox(RemotePayloadV1 {
            endpoint_binding_id: "endpoint-1".to_owned(),
            endpoint_digest: common::digest("endpoint-1"),
            workload_id: "workload-1".to_owned(),
            workload_digest: common::digest("workload-1"),
            credential: ExactRefV1 {
                id: "credential-1".to_owned(),
                revision: 1,
                digest: common::digest("credential-1"),
                expires_unix_nano: expires,
            },
            isolation_profile: "strong".to_owned(),
            inspection_target: None,
        }),
    )
}

#[tokio::test]
async fn socket_remote_connector_preserves_exact_request_and_result() {
    let temporary = tempfile::tempdir().unwrap_or_else(|error| panic!("tempdir: {error}"));
    let socket = temporary.path().join("remote.sock");
    let listener = UnixListener::bind(&socket).unwrap_or_else(|error| panic!("bind: {error}"));
    let server = tokio::spawn(async move {
        let (mut stream, _) = listener
            .accept()
            .await
            .unwrap_or_else(|error| panic!("accept: {error}"));
        let envelope: RemoteConnectorRequestV1 = read_frame(&mut stream)
            .await
            .unwrap_or_else(|error| panic!("request: {error}"));
        envelope
            .validate()
            .unwrap_or_else(|error| panic!("validate request: {error}"));
        assert_eq!(envelope.operation, RemoteOperationV1::ExecutePrepared);
        let result = provider_result(&envelope.request, "remote-complete")
            .unwrap_or_else(|error| panic!("result: {error}"));
        let response = RemoteConnectorResponseV1::seal(envelope.digest, Some(result), None)
            .unwrap_or_else(|error| panic!("response: {error}"));
        write_frame(&mut stream, &response)
            .await
            .unwrap_or_else(|error| panic!("write: {error}"));
    });
    let transport = SocketRemoteTransport::new(socket, Uid::current().as_raw())
        .unwrap_or_else(|error| panic!("transport: {error}"));
    let result = transport
        .invoke(RemoteOperationV1::ExecutePrepared, &remote_request())
        .await
        .unwrap_or_else(|error| panic!("invoke: {error}"));
    assert_eq!(result.observation.state, "remote-complete");
    server
        .await
        .unwrap_or_else(|error| panic!("server: {error}"));
}

#[tokio::test]
async fn socket_remote_connector_rejects_cross_request_reply() {
    let temporary = tempfile::tempdir().unwrap_or_else(|error| panic!("tempdir: {error}"));
    let socket = temporary.path().join("remote.sock");
    let listener = UnixListener::bind(&socket).unwrap_or_else(|error| panic!("bind: {error}"));
    let server = tokio::spawn(async move {
        let (mut stream, _) = listener
            .accept()
            .await
            .unwrap_or_else(|error| panic!("accept: {error}"));
        let envelope: RemoteConnectorRequestV1 = read_frame(&mut stream)
            .await
            .unwrap_or_else(|error| panic!("request: {error}"));
        let result = provider_result(&envelope.request, "remote-complete")
            .unwrap_or_else(|error| panic!("result: {error}"));
        let response =
            RemoteConnectorResponseV1::seal(common::digest("another-request"), Some(result), None)
                .unwrap_or_else(|error| panic!("response: {error}"));
        write_frame(&mut stream, &response)
            .await
            .unwrap_or_else(|error| panic!("write: {error}"));
    });
    let transport = SocketRemoteTransport::new(socket, Uid::current().as_raw())
        .unwrap_or_else(|error| panic!("transport: {error}"));
    let error = common::must_error(
        transport
            .invoke(RemoteOperationV1::ExecutePrepared, &remote_request())
            .await,
    );
    assert_eq!(error.reason, ClosedReason::BindingDrift);
    server
        .await
        .unwrap_or_else(|error| panic!("server: {error}"));
}

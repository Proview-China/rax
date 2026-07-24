mod common;

use praxis_sandbox_dataplane::contract::{CurrentReadResponseV1, EnforcementPhaseV1};
use praxis_sandbox_dataplane::enforcer::CurrentFactsReader;
use praxis_sandbox_dataplane::ipc::{SocketCurrentFactsReader, read_frame, write_frame};
use tokio::net::UnixListener;

#[tokio::test]
async fn socket_current_reader_round_trips_exact_request_and_authorization() {
    let temporary = tempfile::tempdir().unwrap_or_else(|error| panic!("tempdir: {error}"));
    let path = temporary.path().join("current.sock");
    let listener = UnixListener::bind(&path).unwrap_or_else(|error| panic!("listen: {error}"));
    let expected = common::request(EnforcementPhaseV1::Prepare);
    let expected_for_server = expected.clone();
    let server = tokio::spawn(async move {
        let (mut stream, _) = listener
            .accept()
            .await
            .unwrap_or_else(|error| panic!("accept: {error}"));
        let actual = read_frame(&mut stream)
            .await
            .unwrap_or_else(|error| panic!("read request: {error}"));
        assert_eq!(actual, expected_for_server);
        let authorization = common::current_for(&actual)
            .unwrap_or_else(|error| panic!("current authorization: {error}"));
        write_frame(
            &mut stream,
            &CurrentReadResponseV1 {
                authorization: Some(authorization),
                error: None,
            },
        )
        .await
        .unwrap_or_else(|error| panic!("write response: {error}"));
    });
    let reader = SocketCurrentFactsReader::new(path, nix::unistd::Uid::effective().as_raw());
    let authorization = reader
        .inspect_current(&expected)
        .await
        .unwrap_or_else(|error| panic!("inspect current: {error}"));
    authorization
        .validate_against(
            &expected,
            praxis_sandbox_dataplane::contract::now_unix_nano(),
        )
        .unwrap_or_else(|error| panic!("validate authorization: {error}"));
    server
        .await
        .unwrap_or_else(|error| panic!("server join: {error}"));
}

mod common;

use std::io::{Read, Write};
use std::os::unix::net::UnixListener;
use std::thread;
use std::time::Duration;

use nix::unistd::Uid;
use praxis_sandbox_dataplane::contract::{ExactRefV1, WasmCapabilityBindingV1, now_unix_nano};
use praxis_sandbox_dataplane::error::ClosedReason;
use praxis_sandbox_dataplane::wasm::WasmCapabilityHost;
use praxis_sandbox_dataplane::wasm_capability_ipc::{
    SocketWasmCapabilityHost, WasmCapabilityInvokeRequestV1, WasmCapabilityInvokeResponseV1,
};

fn binding() -> WasmCapabilityBindingV1 {
    WasmCapabilityBindingV1 {
        name: "database.query".to_owned(),
        grant: ExactRefV1 {
            id: "grant-db-query".to_owned(),
            revision: 1,
            digest: common::digest("grant-db-query"),
            expires_unix_nano: now_unix_nano() + 30_000_000_000,
        },
        request_schema: "praxis.tool/database-query-request/v1".to_owned(),
        response_schema: "praxis.tool/database-query-response/v1".to_owned(),
        max_request_bytes: 4_096,
        max_response_bytes: 4_096,
    }
}

#[test]
fn socket_capability_gateway_binds_exact_request_and_peer_uid() {
    let temporary = tempfile::tempdir().unwrap_or_else(|error| panic!("tempdir: {error}"));
    let socket = temporary.path().join("capability.sock");
    let listener = UnixListener::bind(&socket).unwrap_or_else(|error| panic!("bind: {error}"));
    let server = thread::spawn(move || {
        let (mut stream, _) = listener
            .accept()
            .unwrap_or_else(|error| panic!("accept: {error}"));
        let request: WasmCapabilityInvokeRequestV1 = read_frame(&mut stream);
        request
            .validate()
            .unwrap_or_else(|error| panic!("request: {error}"));
        assert_eq!(request.binding.name, "database.query");
        assert_eq!(request.request, r#"{"sql":"select 1"}"#);
        let response = WasmCapabilityInvokeResponseV1::seal(
            request.digest,
            Some(r#"{"rows":[[1]]}"#.to_owned()),
            None,
        )
        .unwrap_or_else(|error| panic!("response: {error}"));
        write_frame(&mut stream, &response);
    });
    let host =
        SocketWasmCapabilityHost::new(socket, Uid::current().as_raw(), Duration::from_secs(1))
            .unwrap_or_else(|error| panic!("host: {error}"));
    let result = host
        .invoke(&binding(), r#"{"sql":"select 1"}"#)
        .unwrap_or_else(|error| panic!("invoke: {error}"));
    assert_eq!(result, r#"{"rows":[[1]]}"#);
    server
        .join()
        .unwrap_or_else(|_| panic!("server thread panicked"));
}

#[test]
fn socket_capability_gateway_rejects_cross_request_reply() {
    let temporary = tempfile::tempdir().unwrap_or_else(|error| panic!("tempdir: {error}"));
    let socket = temporary.path().join("capability.sock");
    let listener = UnixListener::bind(&socket).unwrap_or_else(|error| panic!("bind: {error}"));
    let server = thread::spawn(move || {
        let (mut stream, _) = listener
            .accept()
            .unwrap_or_else(|error| panic!("accept: {error}"));
        let _: WasmCapabilityInvokeRequestV1 = read_frame(&mut stream);
        let response = WasmCapabilityInvokeResponseV1::seal(
            common::digest("another-request"),
            Some("{}".to_owned()),
            None,
        )
        .unwrap_or_else(|error| panic!("response: {error}"));
        write_frame(&mut stream, &response);
    });
    let host =
        SocketWasmCapabilityHost::new(socket, Uid::current().as_raw(), Duration::from_secs(1))
            .unwrap_or_else(|error| panic!("host: {error}"));
    let error = common::must_error(host.invoke(&binding(), "{}"));
    assert_eq!(error.reason, ClosedReason::BindingDrift);
    server
        .join()
        .unwrap_or_else(|_| panic!("server thread panicked"));
}

fn read_frame<T: for<'de> serde::Deserialize<'de>>(
    stream: &mut std::os::unix::net::UnixStream,
) -> T {
    let mut length = [0_u8; 4];
    stream
        .read_exact(&mut length)
        .unwrap_or_else(|error| panic!("read length: {error}"));
    let mut payload = vec![0_u8; u32::from_be_bytes(length) as usize];
    stream
        .read_exact(&mut payload)
        .unwrap_or_else(|error| panic!("read payload: {error}"));
    serde_json::from_slice(&payload).unwrap_or_else(|error| panic!("decode: {error}"))
}

fn write_frame<T: serde::Serialize>(stream: &mut std::os::unix::net::UnixStream, value: &T) {
    let payload = serde_json::to_vec(value).unwrap_or_else(|error| panic!("encode: {error}"));
    let frame_len = u32::try_from(payload.len())
        .unwrap_or_else(|error| panic!("test frame exceeds u32: {error}"));
    stream
        .write_all(&frame_len.to_be_bytes())
        .unwrap_or_else(|error| panic!("write length: {error}"));
    stream
        .write_all(&payload)
        .unwrap_or_else(|error| panic!("write payload: {error}"));
}

mod common;

use std::fs;
use std::sync::Arc;
use std::sync::atomic::{AtomicUsize, Ordering};

use praxis_sandbox_dataplane::contract::{
    EnforcementPhaseV1, ExactRefV1, ProviderInspectionTargetV1, ProviderPayloadV1,
    WasmCapabilityBindingV1, WasmPayloadV1, now_unix_nano,
};
use praxis_sandbox_dataplane::error::{ClosedReason, Result};
use praxis_sandbox_dataplane::provider::Provider;
use praxis_sandbox_dataplane::wasm::{WasmCapabilityHost, WasmProvider};
use sha2::{Digest as _, Sha256};

const COMPONENT: &str = r#"
(component
  (core module $m
    (func (export "run") (result i32) i32.const 7))
  (core instance $i (instantiate $m))
  (func (export "run") (result u32)
    (canon lift (core func $i "run"))))
"#;

const CAPABILITY_COMPONENT: &str = r#"
(component
  (import "invoke" (func $invoke
    (param "capability" string)
    (param "request" string)
    (result (result string (error string)))))
  (core module $m
    (func (export "run") (result i32) i32.const 9))
  (core instance $i (instantiate $m))
  (func (export "run") (result u32)
    (canon lift (core func $i "run"))))
"#;

struct CapabilityHostProbe {
    calls: AtomicUsize,
}

impl WasmCapabilityHost for CapabilityHostProbe {
    fn invoke(&self, _binding: &WasmCapabilityBindingV1, _request: &str) -> Result<String> {
        self.calls.fetch_add(1, Ordering::SeqCst);
        Ok("{}".to_owned())
    }
}

#[tokio::test]
async fn real_component_prepare_and_execute() {
    let temporary = tempfile::tempdir().unwrap_or_else(|error| panic!("tempdir: {error}"));
    let path = temporary.path().join("component.wat");
    fs::write(&path, COMPONENT).unwrap_or_else(|error| panic!("write component: {error}"));
    let digest = format!(
        "sha256:{}",
        hex::encode(Sha256::digest(COMPONENT.as_bytes()))
    );
    let payload = ProviderPayloadV1::WasmtimeComponent(WasmPayloadV1 {
        component_digest: digest,
        ..common::wasm_payload()
    });
    let provider = WasmProvider::new(common::bindings(path));
    let prepare = common::request_with_payload(EnforcementPhaseV1::Prepare, payload.clone());
    let execute = common::request_with_payload(EnforcementPhaseV1::Execute, payload);
    let prepared = provider
        .prepare(&prepare)
        .await
        .unwrap_or_else(|error| panic!("prepare: {error}"));
    assert_eq!(prepared.observation.state, "prepared");
    let executed = provider
        .execute_prepared(&execute)
        .await
        .unwrap_or_else(|error| panic!("execute: {error}"));
    assert_eq!(executed.observation.state, "exited:7");
}

#[tokio::test]
async fn independently_governed_inspect_targets_the_original_wasm_attempt() {
    let temporary = tempfile::tempdir().unwrap_or_else(|error| panic!("tempdir: {error}"));
    let path = temporary.path().join("component.wat");
    fs::write(&path, COMPONENT).unwrap_or_else(|error| panic!("write component: {error}"));
    let component_digest = format!(
        "sha256:{}",
        hex::encode(Sha256::digest(COMPONENT.as_bytes()))
    );
    let original_payload = ProviderPayloadV1::WasmtimeComponent(WasmPayloadV1 {
        component_digest: component_digest.clone(),
        ..common::wasm_payload()
    });
    let provider = WasmProvider::new(common::bindings(path));
    let original_prepare =
        common::request_with_payload(EnforcementPhaseV1::Prepare, original_payload.clone());
    provider
        .prepare(&original_prepare)
        .await
        .unwrap_or_else(|error| panic!("prepare original: {error}"));
    let original_execute =
        common::request_with_payload(EnforcementPhaseV1::Execute, original_payload);
    let executed = provider
        .execute_prepared(&original_execute)
        .await
        .unwrap_or_else(|error| panic!("execute original: {error}"));

    let target = ProviderInspectionTargetV1 {
        original_effect_kind: "praxis.sandbox/open".to_owned(),
        original_attempt_id: original_execute.attempt_id.clone(),
        provider_attempt: executed.attempt,
        original_request_digest: original_execute.digest.clone(),
        original_payload_digest: original_execute.payload_digest.clone(),
    };
    let inspect_payload = ProviderPayloadV1::WasmtimeComponent(WasmPayloadV1 {
        component_digest,
        inspection_target: Some(target),
        ..common::wasm_payload()
    });
    let inspect_prepare = inspect_request(EnforcementPhaseV1::Prepare, inspect_payload.clone());
    provider
        .prepare(&inspect_prepare)
        .await
        .unwrap_or_else(|error| panic!("prepare inspect: {error}"));
    let inspect_execute = inspect_request(EnforcementPhaseV1::Execute, inspect_payload);
    let inspected = provider
        .inspect(&inspect_execute)
        .await
        .unwrap_or_else(|error| panic!("inspect original: {error}"));
    assert_eq!(inspected.observation.state, "exited:7");
    assert_ne!(inspect_execute.attempt_id, original_execute.attempt_id);
}

#[tokio::test]
async fn digest_drift_is_rejected_before_compilation() {
    let temporary = tempfile::tempdir().unwrap_or_else(|error| panic!("tempdir: {error}"));
    let path = temporary.path().join("component.wat");
    fs::write(&path, COMPONENT).unwrap_or_else(|error| panic!("write component: {error}"));
    let provider = WasmProvider::new(common::bindings(path));
    let request = common::request(EnforcementPhaseV1::Prepare);
    let error = common::must_error(provider.prepare(&request).await);
    assert_eq!(error.reason, ClosedReason::InvalidDigest);
}

#[tokio::test]
async fn guest_trap_is_only_a_provider_error() {
    const TRAP_COMPONENT: &str = r#"
    (component
      (core module $m (func (export "run") (result i32) unreachable))
      (core instance $i (instantiate $m))
      (func (export "run") (result u32) (canon lift (core func $i "run"))))
    "#;
    let error = execute_component_text(TRAP_COMPONENT).await;
    assert_eq!(error.reason, ClosedReason::ResourceLimit);
}

#[tokio::test]
async fn unknown_host_import_is_denied_by_empty_capability_linker() {
    const IMPORT_COMPONENT: &str = r#"
    (component
      (import "host-call" (func $host-call))
      (core module $m (func (export "run") (result i32) i32.const 0))
      (core instance $i (instantiate $m))
      (func (export "run") (result u32) (canon lift (core func $i "run"))))
    "#;
    let error = execute_component_text(IMPORT_COMPONENT).await;
    assert_eq!(error.reason, ClosedReason::InvalidContract);
}

#[tokio::test]
async fn sealed_capability_import_requires_trusted_host_and_exact_grant() {
    let temporary = tempfile::tempdir().unwrap_or_else(|error| panic!("tempdir: {error}"));
    let path = temporary.path().join("capability-component.wat");
    fs::write(&path, CAPABILITY_COMPONENT)
        .unwrap_or_else(|error| panic!("write component: {error}"));
    let digest = format!(
        "sha256:{}",
        hex::encode(Sha256::digest(CAPABILITY_COMPONENT.as_bytes()))
    );
    let binding = WasmCapabilityBindingV1 {
        name: "filesystem.read".to_owned(),
        grant: ExactRefV1 {
            id: "grant-1".to_owned(),
            revision: 1,
            digest: common::digest("grant-1"),
            expires_unix_nano: now_unix_nano() + 30_000_000_000,
        },
        request_schema: "praxis.tool/filesystem-read-request/v1".to_owned(),
        response_schema: "praxis.tool/filesystem-read-response/v1".to_owned(),
        max_request_bytes: 4_096,
        max_response_bytes: 65_536,
    };
    let payload = ProviderPayloadV1::WasmtimeComponent(WasmPayloadV1 {
        component_digest: digest,
        capability_bindings: vec![binding],
        ..common::wasm_payload()
    });
    let prepare = common::request_with_payload(EnforcementPhaseV1::Prepare, payload.clone());
    let execute = common::request_with_payload(EnforcementPhaseV1::Execute, payload.clone());
    let denied = WasmProvider::new(common::bindings(path.clone()));
    denied
        .prepare(&prepare)
        .await
        .unwrap_or_else(|error| panic!("prepare denied fixture: {error}"));
    let error = common::must_error(denied.execute_prepared(&execute).await);
    assert_eq!(error.reason, ClosedReason::Unsupported);

    let host = Arc::new(CapabilityHostProbe {
        calls: AtomicUsize::new(0),
    });
    let provider = WasmProvider::new(common::bindings(path)).with_capability_host(host.clone());
    provider
        .prepare(&prepare)
        .await
        .unwrap_or_else(|error| panic!("prepare capability: {error}"));
    let result = provider
        .execute_prepared(&execute)
        .await
        .unwrap_or_else(|error| panic!("execute capability: {error}"));
    assert_eq!(result.observation.state, "exited:9");
    assert_eq!(host.calls.load(Ordering::SeqCst), 0);
}

#[tokio::test]
async fn expired_capability_grant_fails_before_component_execution() {
    let temporary = tempfile::tempdir().unwrap_or_else(|error| panic!("tempdir: {error}"));
    let path = temporary.path().join("component.wat");
    fs::write(&path, COMPONENT).unwrap_or_else(|error| panic!("write component: {error}"));
    let digest = format!(
        "sha256:{}",
        hex::encode(Sha256::digest(COMPONENT.as_bytes()))
    );
    let payload = ProviderPayloadV1::WasmtimeComponent(WasmPayloadV1 {
        component_digest: digest,
        capability_bindings: vec![WasmCapabilityBindingV1 {
            name: "filesystem.read".to_owned(),
            grant: ExactRefV1 {
                id: "expired-grant".to_owned(),
                revision: 1,
                digest: common::digest("expired-grant"),
                expires_unix_nano: now_unix_nano(),
            },
            request_schema: "request/v1".to_owned(),
            response_schema: "response/v1".to_owned(),
            max_request_bytes: 1,
            max_response_bytes: 1,
        }],
        ..common::wasm_payload()
    });
    let provider = WasmProvider::new(common::bindings(path)).with_capability_host(Arc::new(
        CapabilityHostProbe {
            calls: AtomicUsize::new(0),
        },
    ));
    let prepare = common::request_with_payload(EnforcementPhaseV1::Prepare, payload.clone());
    provider
        .prepare(&prepare)
        .await
        .unwrap_or_else(|error| panic!("prepare expired fixture: {error}"));
    let execute = common::request_with_payload(EnforcementPhaseV1::Execute, payload);
    let error = common::must_error(provider.execute_prepared(&execute).await);
    assert_eq!(error.reason, ClosedReason::CurrentExpired);
}

#[tokio::test]
async fn fuel_exhaustion_stops_infinite_guest() {
    const LOOP_COMPONENT: &str = r#"
    (component
      (core module $m
        (func (export "run") (result i32)
          (loop $forever br $forever)
          i32.const 0))
      (core instance $i (instantiate $m))
      (func (export "run") (result u32) (canon lift (core func $i "run"))))
    "#;
    let error = execute_component_text(LOOP_COMPONENT).await;
    assert_eq!(error.reason, ClosedReason::ResourceLimit);
}

async fn execute_component_text(component: &str) -> praxis_sandbox_dataplane::ClosedError {
    let temporary = tempfile::tempdir().unwrap_or_else(|error| panic!("tempdir: {error}"));
    let path = temporary.path().join("component.wat");
    fs::write(&path, component).unwrap_or_else(|error| panic!("write component: {error}"));
    let digest = format!(
        "sha256:{}",
        hex::encode(Sha256::digest(component.as_bytes()))
    );
    let payload = ProviderPayloadV1::WasmtimeComponent(WasmPayloadV1 {
        component_digest: digest,
        fuel: 10_000,
        epoch_deadline_ticks: 5,
        ..common::wasm_payload()
    });
    let provider = WasmProvider::new(common::bindings(path));
    let prepare = common::request_with_payload(EnforcementPhaseV1::Prepare, payload.clone());
    provider
        .prepare(&prepare)
        .await
        .unwrap_or_else(|error| panic!("prepare: {error}"));
    let execute = common::request_with_payload(EnforcementPhaseV1::Execute, payload);
    common::must_error(provider.execute_prepared(&execute).await)
}

fn inspect_request(
    phase: EnforcementPhaseV1,
    payload: ProviderPayloadV1,
) -> praxis_sandbox_dataplane::contract::DispatchRequestV1 {
    let mut request = common::request(phase);
    request.payload = payload;
    request.request_id = format!("inspect-{phase:?}");
    request.effect_kind = "praxis.sandbox/inspect".to_owned();
    request.effect_id = "inspect-effect-1".to_owned();
    request.attempt_id = "inspect-attempt-1".to_owned();
    request.sandbox_attempt.id = request.attempt_id.clone();
    request.sandbox_attempt.digest = common::digest("inspect-attempt-1");
    request.runtime_enforcement.effect_id = request.effect_id.clone();
    request.runtime_enforcement.attempt_id = request.attempt_id.clone();
    request.runtime_enforcement.receipt_digest = common::digest(&format!("inspect-{phase:?}"));
    request
        .seal()
        .unwrap_or_else(|error| panic!("inspect request: {error}"))
}

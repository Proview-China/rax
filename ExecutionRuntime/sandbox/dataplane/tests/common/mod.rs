#![allow(dead_code)]

use std::collections::BTreeMap;
use std::sync::Arc;

use async_trait::async_trait;
use praxis_sandbox_dataplane::ClosedError;
use praxis_sandbox_dataplane::contract::{
    CurrentAuthorizationV1, DispatchRequestV1, EnforcementPhaseV1, ExactRefV1, ExecutionBindingV1,
    ProviderBindingV1, ProviderPayloadV1, RuntimeEnforcementRefV1, SandboxProjectionRefV1,
    WasmPayloadV1, canonical_digest, now_unix_nano,
};
use praxis_sandbox_dataplane::enforcer::CurrentFactsReader;
use praxis_sandbox_dataplane::error::Result;

pub struct ExactReader;

#[async_trait]
impl CurrentFactsReader for ExactReader {
    async fn inspect_current(&self, request: &DispatchRequestV1) -> Result<CurrentAuthorizationV1> {
        current_for(request)
    }
}

pub fn request(phase: EnforcementPhaseV1) -> DispatchRequestV1 {
    request_with_payload(phase, ProviderPayloadV1::WasmtimeComponent(wasm_payload()))
}

pub fn request_with_payload(
    phase: EnforcementPhaseV1,
    payload: ProviderPayloadV1,
) -> DispatchRequestV1 {
    let expires = now_unix_nano() + 60_000_000_000;
    DispatchRequestV1 {
        contract_version: String::new(),
        request_id: format!("request-{phase:?}"),
        phase,
        effect_kind: "praxis.sandbox/open".to_owned(),
        operation_digest: digest("operation"),
        effect_id: "effect-1".to_owned(),
        intent_revision: 1,
        intent_digest: digest("intent"),
        attempt_id: "attempt-1".to_owned(),
        tenant_id: "tenant-1".to_owned(),
        provider_binding: provider_binding(),
        sandbox_attempt: exact("attempt-1", expires),
        execution_binding: execution_binding(expires),
        runtime_enforcement: RuntimeEnforcementRefV1 {
            operation_digest: digest("operation"),
            effect_id: "effect-1".to_owned(),
            permit_id: "permit-1".to_owned(),
            attempt_id: "attempt-1".to_owned(),
            phase,
            receipt_digest: digest("runtime-enforcement"),
            journal_revision: if phase == EnforcementPhaseV1::Prepare {
                1
            } else {
                2
            },
            expires_unix_nano: expires,
        },
        runtime_current_query: serde_json::json!({"fixture": "current-query"}),
        runtime_current_query_digest: String::new(),
        requested_not_after_unix_nano: expires,
        payload_schema: "praxis.sandbox/provider-payload/v1".to_owned(),
        payload_digest: String::new(),
        payload_revision: 1,
        payload,
        digest: String::new(),
    }
    .seal()
    .unwrap_or_else(|error| panic!("fixture must seal: {error}"))
}

pub fn current_for(request: &DispatchRequestV1) -> Result<CurrentAuthorizationV1> {
    let now = now_unix_nano();
    let mut current = CurrentAuthorizationV1 {
        contract_version: request.contract_version.clone(),
        request_digest: request.digest.clone(),
        operation_digest: request.operation_digest.clone(),
        effect_id: request.effect_id.clone(),
        attempt_id: request.attempt_id.clone(),
        phase: request.phase,
        provider_binding: request.provider_binding.clone(),
        sandbox_projection: SandboxProjectionRefV1 {
            revision: 1,
            digest: digest("sandbox-projection"),
            expires_unix_nano: request.requested_not_after_unix_nano,
        },
        execution_binding: request.execution_binding.clone(),
        runtime_enforcement: request.runtime_enforcement.clone(),
        checked_unix_nano: now,
        expires_unix_nano: request.requested_not_after_unix_nano,
        digest: String::new(),
    };
    current.digest = current.calculate_digest()?;
    Ok(current)
}

pub fn wasm_payload() -> WasmPayloadV1 {
    WasmPayloadV1 {
        component_path_binding_id: "component-1".to_owned(),
        component_digest: digest("component-placeholder"),
        world: "praxis:sandbox/capability@1.0.0".to_owned(),
        export: "run".to_owned(),
        fuel: 100_000,
        epoch_deadline_ticks: 100,
        memory_limit_bytes: 16 * 1024 * 1024,
        table_elements_limit: 1_024,
        instance_limit: 8,
        capability_bindings: Vec::new(),
        inspection_target: None,
    }
}

pub fn exact(id: &str, expires: i64) -> ExactRefV1 {
    ExactRefV1 {
        id: id.to_owned(),
        revision: 1,
        digest: digest(id),
        expires_unix_nano: expires,
    }
}

pub fn provider_binding() -> ProviderBindingV1 {
    let mut binding = ProviderBindingV1 {
        binding_set_id: "bindings-1".to_owned(),
        binding_set_revision: 1,
        component_id: "sandbox-provider".to_owned(),
        manifest_digest: digest("manifest"),
        artifact_digest: digest("artifact"),
        capability: "sandbox.execute".to_owned(),
        digest: String::new(),
    };
    binding.digest = binding
        .calculate_digest()
        .unwrap_or_else(|error| panic!("binding digest: {error}"));
    binding
}

pub fn execution_binding(expires: i64) -> ExecutionBindingV1 {
    ExecutionBindingV1 {
        tenant_id: "tenant-1".to_owned(),
        instance_id: "instance-1".to_owned(),
        instance_epoch: 1,
        lease_id: "lease-1".to_owned(),
        lease_epoch: 1,
        fence_epoch: 1,
        scope_digest: digest("scope"),
        observed_revision: 1,
        expires_unix_nano: expires,
    }
}

pub fn digest(value: &str) -> String {
    canonical_digest("fixture", &value)
        .unwrap_or_else(|error| panic!("fixture digest must work: {error}"))
}

pub fn reader() -> Arc<ExactReader> {
    Arc::new(ExactReader)
}

pub fn bindings(path: std::path::PathBuf) -> BTreeMap<String, std::path::PathBuf> {
    BTreeMap::from([("component-1".to_owned(), path)])
}

pub fn must_error<T>(result: Result<T>) -> ClosedError {
    match result {
        Ok(_) => panic!("expected a closed error"),
        Err(error) => error,
    }
}

use std::collections::BTreeMap;
use std::path::{Path, PathBuf};
use std::sync::Arc;
use std::time::Duration;

use async_trait::async_trait;
use sha2::{Digest as _, Sha256};
use tokio::fs;
use tokio::sync::Mutex;
use wasmtime::component::{Component, Linker};
use wasmtime::{Config, Engine, Store, StoreLimits, StoreLimitsBuilder};

use crate::checkpoint::CheckpointSource;
use crate::contract::{
    DispatchRequestV1, ProviderKindV1, ProviderPayloadV1, WasmCapabilityBindingV1, WasmPayloadV1,
    now_unix_nano,
};
use crate::error::{ClosedError, ClosedReason, Result};
use crate::provider::{Provider, ProviderResult, provider_result};

#[derive(Clone)]
struct PreparedWasm {
    engine: Engine,
    component: Component,
    payload_digest: String,
    state: String,
}

struct StoreState {
    limits: StoreLimits,
    capability_host: Option<Arc<dyn WasmCapabilityHost>>,
    capabilities: BTreeMap<String, WasmCapabilityBindingV1>,
}

pub trait WasmCapabilityHost: Send + Sync {
    fn invoke(&self, binding: &WasmCapabilityBindingV1, request: &str) -> Result<String>;
}

pub struct WasmProvider {
    component_bindings: BTreeMap<String, PathBuf>,
    attempts: Arc<Mutex<BTreeMap<String, PreparedWasm>>>,
    capability_host: Option<Arc<dyn WasmCapabilityHost>>,
}

impl WasmProvider {
    #[must_use]
    pub fn new(component_bindings: BTreeMap<String, PathBuf>) -> Self {
        Self {
            component_bindings,
            attempts: Arc::new(Mutex::new(BTreeMap::new())),
            capability_host: None,
        }
    }

    #[must_use]
    pub fn with_capability_host(mut self, host: Arc<dyn WasmCapabilityHost>) -> Self {
        self.capability_host = Some(host);
        self
    }

    async fn prepare_exact(&self, request: &DispatchRequestV1) -> Result<()> {
        let payload = wasm_payload(request)?;
        let prepared = self.validate_binding(payload).await?;
        let mut attempts = self.attempts.lock().await;
        match attempts.get(&request.attempt_id) {
            Some(existing) if existing.payload_digest == request.payload_digest => Ok(()),
            Some(_) => Err(ClosedError::new(
                ClosedReason::Conflict,
                "WASM attempt already exists with a different payload",
            )),
            None => {
                attempts.insert(
                    request.attempt_id.clone(),
                    PreparedWasm {
                        engine: prepared.0,
                        component: prepared.1,
                        payload_digest: request.payload_digest.clone(),
                        state: "prepared".to_owned(),
                    },
                );
                Ok(())
            }
        }
    }

    async fn validate_binding(&self, payload: &WasmPayloadV1) -> Result<(Engine, Component)> {
        let Some(path) = self
            .component_bindings
            .get(&payload.component_path_binding_id)
        else {
            return Err(ClosedError::new(
                ClosedReason::NotFoundObservation,
                "WASM component binding was not found",
            ));
        };
        ensure_regular_file(path).await?;
        let bytes = fs::read(path).await.map_err(ClosedError::internal)?;
        let digest = format!("sha256:{}", hex::encode(Sha256::digest(&bytes)));
        if digest != payload.component_digest {
            return Err(ClosedError::new(
                ClosedReason::InvalidDigest,
                "WASM component digest drifted",
            ));
        }
        let engine = build_engine()?;
        let component = Component::new(&engine, &bytes).map_err(|_| {
            ClosedError::new(
                ClosedReason::InvalidContract,
                "WASM component validation failed",
            )
        })?;
        Ok((engine, component))
    }

    async fn prepared(&self, request: &DispatchRequestV1) -> Result<PreparedWasm> {
        if let Some(prepared) = self.attempts.lock().await.get(&request.attempt_id).cloned() {
            if prepared.payload_digest != request.payload_digest {
                return Err(ClosedError::new(
                    ClosedReason::Conflict,
                    "WASM prepared payload drifted",
                ));
            }
            return Ok(prepared);
        }
        // A process restart may evict compiled code. Recompilation is local and
        // side-effect free; the durable journal still proves prepare completed.
        self.prepare_exact(request).await?;
        self.attempts
            .lock()
            .await
            .get(&request.attempt_id)
            .cloned()
            .ok_or_else(|| ClosedError::new(ClosedReason::Internal, "WASM prepare disappeared"))
    }
}

#[async_trait]
impl Provider for WasmProvider {
    fn kind(&self) -> ProviderKindV1 {
        ProviderKindV1::WasmtimeComponent
    }

    async fn prepare(&self, request: &DispatchRequestV1) -> Result<ProviderResult> {
        self.prepare_exact(request).await?;
        provider_result(request, "prepared")
    }

    async fn execute_prepared(&self, request: &DispatchRequestV1) -> Result<ProviderResult> {
        if request.effect_kind == "praxis.sandbox/allocate" {
            self.prepared(request).await?;
            if let Some(current) = self.attempts.lock().await.get_mut(&request.attempt_id) {
                current.state = "allocated".to_owned();
            }
            return provider_result(request, "allocated");
        }
        if request.effect_kind != "praxis.sandbox/activate"
            && request.effect_kind != "praxis.sandbox/open"
        {
            return Err(ClosedError::new(
                ClosedReason::Unsupported,
                "WASM effect has no execute-prepared implementation",
            ));
        }
        let payload = wasm_payload(request)?.clone();
        validate_capability_currents(&payload, request)?;
        let prepared = self.prepared(request).await?;
        {
            let mut attempts = self.attempts.lock().await;
            let current = attempts.get_mut(&request.attempt_id).ok_or_else(|| {
                ClosedError::new(ClosedReason::ProviderUnknown, "WASM attempt is unavailable")
            })?;
            if current.state != "prepared" {
                return Err(ClosedError::new(
                    ClosedReason::Conflict,
                    "WASM attempt is not in prepared state",
                ));
            }
            current.state = "executing".to_owned();
        }
        let engine = prepared.engine.clone();
        let component = prepared.component.clone();
        let capability_host = self.capability_host.clone();
        let deadline_engine = engine.clone();
        let deadline_millis = payload.epoch_deadline_ticks.saturating_mul(10).min(300_000);
        let deadline = tokio::spawn(async move {
            tokio::time::sleep(Duration::from_millis(deadline_millis)).await;
            deadline_engine.increment_epoch();
        });
        let execution = tokio::task::spawn_blocking(move || {
            execute_component(&engine, &component, &payload, capability_host)
        })
        .await;
        deadline.abort();
        let execution = execution.map_err(ClosedError::internal)?;
        let mut attempts = self.attempts.lock().await;
        let current = attempts.get_mut(&request.attempt_id).ok_or_else(|| {
            ClosedError::new(ClosedReason::ProviderUnknown, "WASM attempt disappeared")
        })?;
        match execution {
            Ok(exit_code) => {
                current.state = format!("exited:{exit_code}");
                provider_result(request, &current.state)
            }
            Err(error) => {
                current.state = "trapped".to_owned();
                Err(error)
            }
        }
    }

    async fn inspect(&self, request: &DispatchRequestV1) -> Result<ProviderResult> {
        let target = request.payload.inspection_target();
        if target.is_some_and(|value| value.original_effect_kind == "praxis.sandbox/cleanup") {
            return self.inspect_cleanup(request).await;
        }
        let attempt_id = target.map_or(request.attempt_id.as_str(), |value| {
            value.original_attempt_id.as_str()
        });
        let attempts = self.attempts.lock().await;
        let state = match attempts.get(attempt_id) {
            Some(attempt)
                if target.is_none_or(|value| {
                    attempt.payload_digest == value.original_payload_digest
                }) =>
            {
                attempt.state.clone()
            }
            Some(_) => {
                return Err(ClosedError::new(
                    ClosedReason::Conflict,
                    "WASM inspection target payload drifted",
                ));
            }
            None => "not_found".to_owned(),
        };
        drop(attempts);
        provider_result(request, &state)
    }

    async fn fence(&self, request: &DispatchRequestV1) -> Result<ProviderResult> {
        let mut attempts = self.attempts.lock().await;
        let Some(attempt) = attempts.get_mut(&request.attempt_id) else {
            return provider_result(request, "not_found");
        };
        attempt.engine.increment_epoch();
        attempt.state = "fenced".to_owned();
        provider_result(request, "fenced")
    }

    async fn release(&self, request: &DispatchRequestV1) -> Result<ProviderResult> {
        let removed = self.attempts.lock().await.remove(&request.attempt_id);
        provider_result(
            request,
            if removed.is_some() {
                "released"
            } else {
                "not_found"
            },
        )
    }

    async fn cleanup(&self, request: &DispatchRequestV1) -> Result<ProviderResult> {
        self.attempts.lock().await.remove(&request.attempt_id);
        provider_result(request, "cleanup_absent")
    }

    async fn inspect_cleanup(&self, request: &DispatchRequestV1) -> Result<ProviderResult> {
        let present = self.attempts.lock().await.contains_key(&request.attempt_id);
        provider_result(
            request,
            if present {
                "residual_present"
            } else {
                "cleanup_absent"
            },
        )
    }

    async fn checkpoint_source(&self, request: &DispatchRequestV1) -> Result<CheckpointSource> {
        let payload = wasm_payload(request)?;
        self.validate_binding(payload).await?;
        let path = self
            .component_bindings
            .get(&payload.component_path_binding_id)
            .ok_or_else(|| {
                ClosedError::new(
                    ClosedReason::NotFoundObservation,
                    "WASM checkpoint component binding was not found",
                )
            })?;
        Ok(CheckpointSource::File(path.clone()))
    }
}

fn build_engine() -> Result<Engine> {
    let mut config = Config::new();
    config.wasm_component_model(true);
    config.consume_fuel(true);
    config.epoch_interruption(true);
    config.cranelift_nan_canonicalization(true);
    Engine::new(&config).map_err(ClosedError::internal)
}

fn execute_component(
    engine: &Engine,
    component: &Component,
    payload: &WasmPayloadV1,
    capability_host: Option<Arc<dyn WasmCapabilityHost>>,
) -> Result<u32> {
    let limits = StoreLimitsBuilder::new()
        .memory_size(payload.memory_limit_bytes)
        .table_elements(payload.table_elements_limit)
        .instances(payload.instance_limit)
        .trap_on_grow_failure(true)
        .build();
    if !payload.capability_bindings.is_empty() && capability_host.is_none() {
        return Err(ClosedError::new(
            ClosedReason::Unsupported,
            "WASM capability bindings require a trusted host gateway",
        ));
    }
    let capabilities = payload
        .capability_bindings
        .iter()
        .map(|binding| (binding.name.clone(), binding.clone()))
        .collect();
    let mut store = Store::new(
        engine,
        StoreState {
            limits,
            capability_host,
            capabilities,
        },
    );
    store.limiter(|state| &mut state.limits);
    store.set_fuel(payload.fuel).map_err(|_| {
        ClosedError::new(
            ClosedReason::ResourceLimit,
            "WASM fuel could not be installed",
        )
    })?;
    store.set_epoch_deadline(1);
    store.epoch_deadline_trap();
    let mut linker = Linker::<StoreState>::new(engine);
    if !payload.capability_bindings.is_empty() {
        linker
            .root()
            .func_wrap(
                "invoke",
                |store,
                 (name, request): (String, String)|
                 -> wasmtime::Result<(std::result::Result<String, String>,)> {
                    let Some(binding) = store.data().capabilities.get(&name) else {
                        return Ok((Err("capability is not granted".to_owned()),));
                    };
                    if request.len() as u64 > binding.max_request_bytes {
                        return Ok(
                            (Err("capability request exceeds its sealed limit".to_owned()),),
                        );
                    }
                    let Some(host) = store.data().capability_host.as_ref() else {
                        return Ok((Err("capability host is unavailable".to_owned()),));
                    };
                    match host.invoke(binding, &request) {
                        Ok(response) if response.len() as u64 <= binding.max_response_bytes => {
                            Ok((Ok(response),))
                        }
                        Ok(_) => Ok((Err(
                            "capability response exceeds its sealed limit".to_owned()
                        ),)),
                        Err(error) => Ok((Err(format!("{:?}: {}", error.reason, error.message)),)),
                    }
                },
            )
            .map_err(ClosedError::internal)?;
    }
    let instance = linker.instantiate(&mut store, component).map_err(|_| {
        ClosedError::new(
            ClosedReason::InvalidContract,
            "WASM component imports are not in the capability allowlist",
        )
    })?;
    let run = instance
        .get_typed_func::<(), (u32,)>(&mut store, &payload.export)
        .map_err(|_| {
            ClosedError::new(
                ClosedReason::InvalidContract,
                "WASM component export does not match the sealed WIT contract",
            )
        })?;
    let (exit_code,) = run.call(&mut store, ()).map_err(|_| {
        ClosedError::new(
            ClosedReason::ResourceLimit,
            "WASM execution trapped or exceeded its closed limits",
        )
    })?;
    run.post_return(&mut store).map_err(ClosedError::internal)?;
    Ok(exit_code)
}

fn validate_capability_currents(
    payload: &WasmPayloadV1,
    request: &DispatchRequestV1,
) -> Result<()> {
    let now = now_unix_nano();
    for binding in &payload.capability_bindings {
        binding
            .grant
            .validate_current("WASM capability grant", now)?;
        if binding.grant.expires_unix_nano > request.requested_not_after_unix_nano {
            return Err(ClosedError::new(
                ClosedReason::BindingDrift,
                "WASM capability grant extends the governed request TTL",
            ));
        }
    }
    Ok(())
}

fn wasm_payload(request: &DispatchRequestV1) -> Result<&WasmPayloadV1> {
    let ProviderPayloadV1::WasmtimeComponent(payload) = &request.payload else {
        return Err(ClosedError::new(
            ClosedReason::BindingDrift,
            "non-WASM payload reached Wasmtime provider",
        ));
    };
    Ok(payload)
}

async fn ensure_regular_file(path: &Path) -> Result<()> {
    let metadata = fs::symlink_metadata(path).await.map_err(|_| {
        ClosedError::new(
            ClosedReason::NotFoundObservation,
            "bound artifact is unavailable",
        )
    })?;
    if !metadata.file_type().is_file() || metadata.file_type().is_symlink() {
        return Err(ClosedError::new(
            ClosedReason::InvalidArgument,
            "bound artifact is not a regular file",
        ));
    }
    Ok(())
}

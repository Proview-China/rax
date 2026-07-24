mod common;

use std::sync::Arc;
use std::sync::atomic::{AtomicUsize, Ordering};

use async_trait::async_trait;
use praxis_sandbox_dataplane::contract::{
    CurrentAuthorizationV1, DispatchRequestV1, EnforcementPhaseV1, ProviderKindV1,
};
use praxis_sandbox_dataplane::enforcer::{CurrentFactsReader, DataPlaneEnforcer};
use praxis_sandbox_dataplane::error::{ClosedError, ClosedReason, Result};
use praxis_sandbox_dataplane::journal::AttemptJournal;
use praxis_sandbox_dataplane::provider::{Provider, ProviderResult, provider_result};

struct CountingProvider {
    calls: AtomicUsize,
    release_calls: AtomicUsize,
    cleanup_calls: AtomicUsize,
    fail_unknown: bool,
}

#[async_trait]
impl Provider for CountingProvider {
    fn kind(&self) -> ProviderKindV1 {
        ProviderKindV1::WasmtimeComponent
    }

    async fn prepare(&self, request: &DispatchRequestV1) -> Result<ProviderResult> {
        self.called(request)
    }

    async fn execute_prepared(&self, request: &DispatchRequestV1) -> Result<ProviderResult> {
        self.called(request)
    }

    async fn inspect(&self, request: &DispatchRequestV1) -> Result<ProviderResult> {
        self.called(request)
    }

    async fn fence(&self, request: &DispatchRequestV1) -> Result<ProviderResult> {
        self.called(request)
    }

    async fn release(&self, request: &DispatchRequestV1) -> Result<ProviderResult> {
        self.release_calls.fetch_add(1, Ordering::SeqCst);
        self.called(request)
    }

    async fn cleanup(&self, request: &DispatchRequestV1) -> Result<ProviderResult> {
        self.cleanup_calls.fetch_add(1, Ordering::SeqCst);
        self.called(request)
    }

    async fn inspect_cleanup(&self, request: &DispatchRequestV1) -> Result<ProviderResult> {
        self.called(request)
    }
}

impl CountingProvider {
    fn called(&self, request: &DispatchRequestV1) -> Result<ProviderResult> {
        self.calls.fetch_add(1, Ordering::SeqCst);
        if self.fail_unknown {
            return Err(ClosedError::new(
                ClosedReason::ProviderUnknown,
                "simulated lost reply",
            ));
        }
        provider_result(request, "ok")
    }
}

struct DriftReader;

#[async_trait]
impl CurrentFactsReader for DriftReader {
    async fn inspect_current(&self, request: &DispatchRequestV1) -> Result<CurrentAuthorizationV1> {
        let mut current = common::current_for(request)?;
        current.effect_id = "other-effect".to_owned();
        current.digest = current.calculate_digest()?;
        Ok(current)
    }
}

#[tokio::test]
async fn current_drift_causes_zero_provider_calls() {
    let temporary = tempfile::tempdir().unwrap_or_else(|error| panic!("tempdir: {error}"));
    let journal = Arc::new(
        AttemptJournal::open(temporary.path().join("journal"))
            .await
            .unwrap_or_else(|error| panic!("journal: {error}")),
    );
    let enforcer = DataPlaneEnforcer::new(Arc::new(DriftReader), journal);
    let provider = CountingProvider {
        calls: AtomicUsize::new(0),
        release_calls: AtomicUsize::new(0),
        cleanup_calls: AtomicUsize::new(0),
        fail_unknown: false,
    };
    let error = common::must_error(
        enforcer
            .dispatch(&provider, &common::request(EnforcementPhaseV1::Prepare))
            .await,
    );
    assert_eq!(error.reason, ClosedReason::BindingDrift);
    assert_eq!(provider.calls.load(Ordering::SeqCst), 0);
}

#[tokio::test]
async fn lost_reply_is_not_replayed_and_requires_inspect() {
    let temporary = tempfile::tempdir().unwrap_or_else(|error| panic!("tempdir: {error}"));
    let journal = Arc::new(
        AttemptJournal::open(temporary.path().join("journal"))
            .await
            .unwrap_or_else(|error| panic!("journal: {error}")),
    );
    let enforcer = DataPlaneEnforcer::new(common::reader(), journal);
    let provider = CountingProvider {
        calls: AtomicUsize::new(0),
        release_calls: AtomicUsize::new(0),
        cleanup_calls: AtomicUsize::new(0),
        fail_unknown: true,
    };
    let request = common::request(EnforcementPhaseV1::Prepare);
    let first = enforcer.dispatch(&provider, &request).await;
    assert_eq!(
        common::must_error(first).reason,
        ClosedReason::ProviderUnknown
    );
    let second = enforcer.dispatch(&provider, &request).await;
    assert_eq!(
        common::must_error(second).reason,
        ClosedReason::ProviderUnknown
    );
    assert_eq!(provider.calls.load(Ordering::SeqCst), 1);
}

#[tokio::test]
async fn completed_dispatch_lost_reply_recovers_durable_result_without_provider_call() {
    let temporary = tempfile::tempdir().unwrap_or_else(|error| panic!("tempdir: {error}"));
    let journal = Arc::new(
        AttemptJournal::open(temporary.path().join("journal"))
            .await
            .unwrap_or_else(|error| panic!("journal: {error}")),
    );
    let enforcer = DataPlaneEnforcer::new(common::reader(), journal);
    let provider = CountingProvider {
        calls: AtomicUsize::new(0),
        release_calls: AtomicUsize::new(0),
        cleanup_calls: AtomicUsize::new(0),
        fail_unknown: false,
    };
    let request = common::request(EnforcementPhaseV1::Prepare);
    let dispatched = enforcer
        .dispatch(&provider, &request)
        .await
        .unwrap_or_else(|error| panic!("dispatch: {error}"));
    let inspected = enforcer
        .inspect(&request)
        .await
        .unwrap_or_else(|error| panic!("inspect: {error}"));
    assert_eq!(inspected, dispatched);
    assert_eq!(provider.calls.load(Ordering::SeqCst), 1);
}

#[tokio::test]
async fn execute_requires_durable_completed_prepare() {
    let temporary = tempfile::tempdir().unwrap_or_else(|error| panic!("tempdir: {error}"));
    let journal = Arc::new(
        AttemptJournal::open(temporary.path().join("journal"))
            .await
            .unwrap_or_else(|error| panic!("journal: {error}")),
    );
    let enforcer = DataPlaneEnforcer::new(common::reader(), journal);
    let provider = CountingProvider {
        calls: AtomicUsize::new(0),
        release_calls: AtomicUsize::new(0),
        cleanup_calls: AtomicUsize::new(0),
        fail_unknown: false,
    };
    let error = common::must_error(
        enforcer
            .dispatch(&provider, &common::request(EnforcementPhaseV1::Execute))
            .await,
    );
    assert_eq!(error.reason, ClosedReason::Conflict);
    assert_eq!(provider.calls.load(Ordering::SeqCst), 0);
}

#[tokio::test]
async fn sixty_four_concurrent_dispatches_have_one_provider_winner() {
    let temporary = tempfile::tempdir().unwrap_or_else(|error| panic!("tempdir: {error}"));
    let journal = Arc::new(
        AttemptJournal::open(temporary.path().join("journal"))
            .await
            .unwrap_or_else(|error| panic!("journal: {error}")),
    );
    let enforcer = Arc::new(DataPlaneEnforcer::new(common::reader(), journal));
    let provider = Arc::new(CountingProvider {
        calls: AtomicUsize::new(0),
        release_calls: AtomicUsize::new(0),
        cleanup_calls: AtomicUsize::new(0),
        fail_unknown: false,
    });
    let request = Arc::new(common::request(EnforcementPhaseV1::Prepare));
    let mut tasks = tokio::task::JoinSet::new();
    for _ in 0..64 {
        let enforcer = Arc::clone(&enforcer);
        let provider = Arc::clone(&provider);
        let request = Arc::clone(&request);
        tasks.spawn(async move { enforcer.dispatch(provider.as_ref(), &request).await });
    }
    let mut winners = 0;
    while let Some(joined) = tasks.join_next().await {
        match joined.unwrap_or_else(|error| panic!("task join: {error}")) {
            Ok(_) => winners += 1,
            Err(error) => assert!(
                error.reason == ClosedReason::ProviderUnknown
                    || error.reason == ClosedReason::Conflict
            ),
        }
    }
    assert_eq!(winners, 1);
    assert_eq!(provider.calls.load(Ordering::SeqCst), 1);
}

#[tokio::test]
async fn execute_routes_release_effect_without_using_generic_execute() {
    let temporary = tempfile::tempdir().unwrap_or_else(|error| panic!("tempdir: {error}"));
    let journal = Arc::new(
        AttemptJournal::open(temporary.path().join("journal"))
            .await
            .unwrap_or_else(|error| panic!("journal: {error}")),
    );
    let enforcer = DataPlaneEnforcer::new(common::reader(), journal);
    let provider = CountingProvider {
        calls: AtomicUsize::new(0),
        release_calls: AtomicUsize::new(0),
        cleanup_calls: AtomicUsize::new(0),
        fail_unknown: false,
    };
    let mut prepare = common::request(EnforcementPhaseV1::Prepare);
    prepare.effect_kind = "praxis.sandbox/release".to_owned();
    prepare = prepare
        .seal()
        .unwrap_or_else(|error| panic!("prepare seal: {error}"));
    enforcer
        .dispatch(&provider, &prepare)
        .await
        .unwrap_or_else(|error| panic!("prepare: {error}"));
    let mut execute = common::request(EnforcementPhaseV1::Execute);
    execute.effect_kind = "praxis.sandbox/release".to_owned();
    execute = execute
        .seal()
        .unwrap_or_else(|error| panic!("execute seal: {error}"));
    enforcer
        .dispatch(&provider, &execute)
        .await
        .unwrap_or_else(|error| panic!("execute: {error}"));
    assert_eq!(provider.release_calls.load(Ordering::SeqCst), 1);
    assert_eq!(provider.cleanup_calls.load(Ordering::SeqCst), 0);
}

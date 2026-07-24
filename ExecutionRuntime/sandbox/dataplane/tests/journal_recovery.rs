mod common;

use praxis_sandbox_dataplane::contract::EnforcementPhaseV1;
use praxis_sandbox_dataplane::error::ClosedReason;
use praxis_sandbox_dataplane::journal::AttemptJournal;
use praxis_sandbox_dataplane::provider::provider_result;

#[tokio::test]
async fn started_attempt_survives_restart_as_unknown() {
    let temporary = tempfile::tempdir().unwrap_or_else(|error| panic!("tempdir: {error}"));
    let path = temporary.path().join("journal");
    let request = common::request(EnforcementPhaseV1::Prepare);
    let journal = AttemptJournal::open(&path)
        .await
        .unwrap_or_else(|error| panic!("journal: {error}"));
    journal
        .begin(&request)
        .await
        .unwrap_or_else(|error| panic!("begin: {error}"));
    drop(journal);

    let recovered = AttemptJournal::open(&path)
        .await
        .unwrap_or_else(|error| panic!("recover: {error}"));
    let error = common::must_error(recovered.begin(&request).await);
    assert_eq!(error.reason, ClosedReason::ProviderUnknown);
}

#[tokio::test]
async fn completed_prepare_unlocks_only_matching_execute_payload() {
    let temporary = tempfile::tempdir().unwrap_or_else(|error| panic!("tempdir: {error}"));
    let journal = AttemptJournal::open(temporary.path().join("journal"))
        .await
        .unwrap_or_else(|error| panic!("journal: {error}"));
    let prepare = common::request(EnforcementPhaseV1::Prepare);
    journal
        .begin(&prepare)
        .await
        .unwrap_or_else(|error| panic!("begin: {error}"));
    journal
        .complete(
            &prepare,
            &provider_result(&prepare, "prepared")
                .unwrap_or_else(|error| panic!("result: {error}")),
        )
        .await
        .unwrap_or_else(|error| panic!("complete: {error}"));
    let execute = common::request(EnforcementPhaseV1::Execute);
    journal
        .begin(&execute)
        .await
        .unwrap_or_else(|error| panic!("matching execute: {error}"));
}

#[tokio::test]
async fn completed_result_survives_restart_and_inspect_never_dispatches() {
    let temporary = tempfile::tempdir().unwrap_or_else(|error| panic!("tempdir: {error}"));
    let path = temporary.path().join("journal");
    let request = common::request(EnforcementPhaseV1::Prepare);
    let expected =
        provider_result(&request, "prepared").unwrap_or_else(|error| panic!("result: {error}"));
    let journal = AttemptJournal::open(&path)
        .await
        .unwrap_or_else(|error| panic!("journal: {error}"));
    journal
        .begin(&request)
        .await
        .unwrap_or_else(|error| panic!("begin: {error}"));
    journal
        .complete(&request, &expected)
        .await
        .unwrap_or_else(|error| panic!("complete: {error}"));
    drop(journal);

    let recovered = AttemptJournal::open(&path)
        .await
        .unwrap_or_else(|error| panic!("recover: {error}"));
    let inspected = recovered
        .inspect(&request)
        .await
        .unwrap_or_else(|error| panic!("inspect: {error}"));
    assert_eq!(inspected, expected);
    assert_eq!(
        common::must_error(recovered.begin(&request).await).reason,
        ClosedReason::Conflict
    );
}

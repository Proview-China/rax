mod common;

use praxis_sandbox_dataplane::contract::{EnforcementPhaseV1, canonical_digest};
use praxis_sandbox_dataplane::error::ClosedReason;
use praxis_sandbox_dataplane::journal::{AttemptJournal, WorkspaceReadPhysicalJournalLookupV2};
use praxis_sandbox_dataplane::provider::provider_result;

fn lookup(
    request: &praxis_sandbox_dataplane::DispatchRequestV1,
) -> WorkspaceReadPhysicalJournalLookupV2 {
    let mut lookup = WorkspaceReadPhysicalJournalLookupV2 {
        attempt_id: request.attempt_id.clone(),
        request_digest: request.digest.clone(),
        payload_digest: request.payload_digest.clone(),
        phase: request.phase,
        digest: String::new(),
    };
    lookup.digest = canonical_digest("WorkspaceReadPhysicalJournalLookupV2", &lookup)
        .unwrap_or_else(|error| panic!("lookup digest: {error}"));
    lookup
}

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

#[tokio::test]
async fn started_and_completed_expose_exact_append_only_evidence() {
    let temporary = tempfile::tempdir().unwrap_or_else(|error| panic!("tempdir: {error}"));
    let request = common::request(EnforcementPhaseV1::Prepare);
    let journal = AttemptJournal::open(temporary.path().join("journal"))
        .await
        .unwrap_or_else(|error| panic!("journal: {error}"));
    journal
        .begin(&request)
        .await
        .unwrap_or_else(|error| panic!("begin: {error}"));
    let started = journal
        .inspect_workspace_read_v2(&request)
        .await
        .unwrap_or_else(|error| panic!("inspect started: {error}"));
    assert_eq!(started.journal.attempt_id, request.attempt_id);
    assert_eq!(started.journal.request_digest, request.digest);
    assert_eq!(started.journal.payload_digest, request.payload_digest);
    assert_eq!(started.journal.state, "started");
    assert_eq!(started.journal.revision, 1);
    assert!(started.result.is_none());

    let result =
        provider_result(&request, "prepared").unwrap_or_else(|error| panic!("result: {error}"));
    journal
        .complete(&request, &result)
        .await
        .unwrap_or_else(|error| panic!("complete: {error}"));
    let completed = journal
        .inspect_workspace_read_v2(&request)
        .await
        .unwrap_or_else(|error| panic!("inspect completed: {error}"));
    assert_eq!(completed.journal.state, "completed");
    assert_eq!(completed.journal.revision, 2);
    assert_ne!(
        completed.journal.record_digest,
        started.journal.record_digest
    );
    assert_eq!(completed.result, Some(result));
}

#[tokio::test]
async fn exact_journal_evidence_remains_readable_after_request_expiry() {
    let temporary = tempfile::tempdir().unwrap_or_else(|error| panic!("tempdir: {error}"));
    let mut request = common::request(EnforcementPhaseV1::Prepare);
    let expires = praxis_sandbox_dataplane::contract::now_unix_nano() + 200_000_000;
    request.requested_not_after_unix_nano = expires;
    request.sandbox_attempt.expires_unix_nano = expires;
    request.execution_binding.expires_unix_nano = expires;
    request.runtime_enforcement.expires_unix_nano = expires;
    request = request
        .seal()
        .unwrap_or_else(|error| panic!("short request seal: {error}"));
    let result =
        provider_result(&request, "prepared").unwrap_or_else(|error| panic!("result: {error}"));
    let path = temporary.path().join("journal");
    let journal = AttemptJournal::open(&path)
        .await
        .unwrap_or_else(|error| panic!("journal: {error}"));
    journal
        .begin(&request)
        .await
        .unwrap_or_else(|error| panic!("begin: {error}"));
    journal
        .complete(&request, &result)
        .await
        .unwrap_or_else(|error| panic!("complete: {error}"));
    drop(journal);
    tokio::time::sleep(std::time::Duration::from_millis(250)).await;

    assert!(
        request
            .validate_current(praxis_sandbox_dataplane::contract::now_unix_nano())
            .is_err(),
        "fixture must be expired"
    );
    let reopened = AttemptJournal::open(&path)
        .await
        .unwrap_or_else(|error| panic!("reopen: {error}"));
    let inspected = reopened
        .inspect_workspace_read_v2(&request)
        .await
        .unwrap_or_else(|error| panic!("historical inspect: {error}"));
    assert_eq!(inspected.journal.state, "completed");
    assert_eq!(inspected.result, Some(result));
}

#[tokio::test]
async fn exact_lookup_recovers_started_then_completed_without_provider_replay() {
    let temporary = tempfile::tempdir().unwrap_or_else(|error| panic!("tempdir: {error}"));
    let path = temporary.path().join("journal");
    let journal = AttemptJournal::open(&path)
        .await
        .unwrap_or_else(|error| panic!("journal: {error}"));
    let prepare = common::request(EnforcementPhaseV1::Prepare);
    let prepared = provider_result(&prepare, "prepared")
        .unwrap_or_else(|error| panic!("prepare result: {error}"));
    journal
        .begin(&prepare)
        .await
        .unwrap_or_else(|error| panic!("prepare begin: {error}"));
    journal
        .complete(&prepare, &prepared)
        .await
        .unwrap_or_else(|error| panic!("prepare complete: {error}"));

    let execute = common::request(EnforcementPhaseV1::Execute);
    let exact = lookup(&execute);
    journal
        .begin(&execute)
        .await
        .unwrap_or_else(|error| panic!("execute begin: {error}"));
    let started = journal
        .inspect_workspace_read_lookup_v2(&exact)
        .await
        .unwrap_or_else(|error| panic!("started lookup: {error}"));
    assert_eq!(started.journal.state, "started");
    assert_eq!(started.request, Some(execute.clone()));
    assert!(started.result.is_none());

    let completed_result = provider_result(&execute, "completed")
        .unwrap_or_else(|error| panic!("execute result: {error}"));
    journal
        .complete(&execute, &completed_result)
        .await
        .unwrap_or_else(|error| panic!("execute complete: {error}"));
    drop(journal);

    let reopened = AttemptJournal::open(&path)
        .await
        .unwrap_or_else(|error| panic!("reopen: {error}"));
    let completed = reopened
        .inspect_workspace_read_lookup_v2(&exact)
        .await
        .unwrap_or_else(|error| panic!("completed lookup: {error}"));
    assert_eq!(completed.journal.state, "completed");
    assert_eq!(completed.request, Some(execute));
    assert_eq!(completed.result, Some(completed_result));
}

#[tokio::test]
async fn completed_legacy_record_without_sealed_request_is_evidence_only() {
    let temporary = tempfile::tempdir().unwrap_or_else(|error| panic!("tempdir: {error}"));
    let path = temporary.path().join("journal");
    let journal = AttemptJournal::open(&path)
        .await
        .unwrap_or_else(|error| panic!("journal: {error}"));
    let prepare = common::request(EnforcementPhaseV1::Prepare);
    journal
        .begin(&prepare)
        .await
        .unwrap_or_else(|error| panic!("prepare begin: {error}"));
    journal
        .complete(
            &prepare,
            &provider_result(&prepare, "prepared")
                .unwrap_or_else(|error| panic!("prepare result: {error}")),
        )
        .await
        .unwrap_or_else(|error| panic!("prepare complete: {error}"));

    let execute = common::request(EnforcementPhaseV1::Execute);
    let exact = lookup(&execute);
    journal
        .begin(&execute)
        .await
        .unwrap_or_else(|error| panic!("execute begin: {error}"));
    journal
        .complete(
            &execute,
            &provider_result(&execute, "completed")
                .unwrap_or_else(|error| panic!("execute result: {error}")),
        )
        .await
        .unwrap_or_else(|error| panic!("execute complete: {error}"));
    drop(journal);

    let contents = tokio::fs::read_to_string(&path)
        .await
        .unwrap_or_else(|error| panic!("read journal: {error}"));
    let mut rewritten = String::new();
    for line in contents.lines() {
        let mut record: serde_json::Value =
            serde_json::from_str(line).unwrap_or_else(|error| panic!("decode row: {error}"));
        if record
            .get("request_digest")
            .and_then(serde_json::Value::as_str)
            == Some(execute.digest.as_str())
        {
            record
                .as_object_mut()
                .unwrap_or_else(|| panic!("journal row must be an object"))
                .remove("request");
        }
        rewritten.push_str(
            &serde_json::to_string(&record)
                .unwrap_or_else(|error| panic!("encode legacy row: {error}")),
        );
        rewritten.push('\n');
    }
    tokio::fs::write(&path, rewritten)
        .await
        .unwrap_or_else(|error| panic!("rewrite legacy journal: {error}"));

    let reopened = AttemptJournal::open(&path)
        .await
        .unwrap_or_else(|error| panic!("reopen legacy journal: {error}"));
    let inspection = reopened
        .inspect_workspace_read_lookup_v2(&exact)
        .await
        .unwrap_or_else(|error| panic!("inspect legacy journal: {error}"));
    assert_eq!(inspection.journal.state, "completed");
    assert!(inspection.request.is_none());
    assert!(inspection.result.is_none());
}

#[tokio::test]
async fn exact_lookup_rejects_spliced_axes() {
    let temporary = tempfile::tempdir().unwrap_or_else(|error| panic!("tempdir: {error}"));
    let journal = AttemptJournal::open(temporary.path().join("journal"))
        .await
        .unwrap_or_else(|error| panic!("journal: {error}"));
    let prepare = common::request(EnforcementPhaseV1::Prepare);
    journal
        .begin(&prepare)
        .await
        .unwrap_or_else(|error| panic!("prepare begin: {error}"));
    journal
        .complete(
            &prepare,
            &provider_result(&prepare, "prepared")
                .unwrap_or_else(|error| panic!("prepare result: {error}")),
        )
        .await
        .unwrap_or_else(|error| panic!("prepare complete: {error}"));
    let request = common::request(EnforcementPhaseV1::Execute);
    journal
        .begin(&request)
        .await
        .unwrap_or_else(|error| panic!("execute begin: {error}"));
    let mut spliced = lookup(&request);
    spliced.attempt_id.push_str("-splice");
    spliced.digest = canonical_digest("WorkspaceReadPhysicalJournalLookupV2", &{
        let mut value = spliced.clone();
        value.digest.clear();
        value
    })
    .unwrap_or_else(|error| panic!("spliced digest: {error}"));
    assert_eq!(
        common::must_error(journal.inspect_workspace_read_lookup_v2(&spliced).await).reason,
        ClosedReason::Conflict
    );
}

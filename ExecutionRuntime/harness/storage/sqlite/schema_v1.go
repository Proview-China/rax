package sqlite

const schemaVersionV1 = 1

const schemaV1 = `
CREATE TABLE IF NOT EXISTS harness_state_schema_v1 (
  version INTEGER PRIMARY KEY,
  digest TEXT NOT NULL,
  applied_unix_nano INTEGER NOT NULL CHECK(applied_unix_nano > 0)
);
CREATE TABLE IF NOT EXISTS harness_state_identity_v1 (
  singleton INTEGER PRIMARY KEY CHECK(singleton = 1),
  store_id TEXT NOT NULL UNIQUE,
  database_identity_digest TEXT NOT NULL,
  clock_high_water_unix_nano INTEGER NOT NULL CHECK(clock_high_water_unix_nano > 0)
);
CREATE TABLE IF NOT EXISTS harness_session_history_v4 (
  scope_digest TEXT NOT NULL,
  run_id TEXT NOT NULL,
  session_id TEXT NOT NULL,
  revision INTEGER NOT NULL CHECK(revision > 0),
  digest TEXT NOT NULL,
  updated_unix_nano INTEGER NOT NULL CHECK(updated_unix_nano > 0),
  row_digest TEXT NOT NULL,
  canonical_json BLOB NOT NULL,
  PRIMARY KEY(scope_digest, run_id, session_id, revision)
);
CREATE TABLE IF NOT EXISTS harness_session_current_v4 (
  scope_digest TEXT NOT NULL,
  run_id TEXT NOT NULL,
  session_id TEXT NOT NULL,
  highest_revision INTEGER NOT NULL CHECK(highest_revision > 0),
  digest TEXT NOT NULL,
  PRIMARY KEY(scope_digest, run_id, session_id),
  FOREIGN KEY(scope_digest, run_id, session_id, highest_revision)
    REFERENCES harness_session_history_v4(scope_digest, run_id, session_id, revision)
);
CREATE TABLE IF NOT EXISTS harness_active_scope_v4 (
  scope_digest TEXT PRIMARY KEY,
  run_id TEXT NOT NULL,
  session_id TEXT NOT NULL,
  FOREIGN KEY(scope_digest, run_id, session_id)
    REFERENCES harness_session_current_v4(scope_digest, run_id, session_id)
);
CREATE TABLE IF NOT EXISTS harness_event_candidate_v1 (
  source_component_id TEXT NOT NULL,
  source_epoch INTEGER NOT NULL CHECK(source_epoch > 0),
  source_sequence INTEGER NOT NULL CHECK(source_sequence > 0),
  run_id TEXT NOT NULL,
  observed_unix_nano INTEGER NOT NULL CHECK(observed_unix_nano > 0),
  event_digest TEXT NOT NULL,
  row_digest TEXT NOT NULL,
  canonical_json BLOB NOT NULL,
  PRIMARY KEY(source_component_id, source_epoch, source_sequence)
);
CREATE TABLE IF NOT EXISTS harness_event_source_head_v1 (
  source_component_id TEXT NOT NULL,
  source_epoch INTEGER NOT NULL CHECK(source_epoch > 0),
  highest_sequence INTEGER NOT NULL CHECK(highest_sequence > 0),
  highest_event_digest TEXT NOT NULL,
  last_observed_unix_nano INTEGER NOT NULL CHECK(last_observed_unix_nano > 0),
  PRIMARY KEY(source_component_id, source_epoch),
  FOREIGN KEY(source_component_id, source_epoch, highest_sequence)
    REFERENCES harness_event_candidate_v1(source_component_id, source_epoch, source_sequence)
);
CREATE TABLE IF NOT EXISTS harness_session_event_proof_history_v1 (
  store_id TEXT NOT NULL,
  revision INTEGER NOT NULL CHECK(revision > 0),
  digest TEXT NOT NULL,
  expires_unix_nano INTEGER NOT NULL,
  row_digest TEXT NOT NULL,
  canonical_json BLOB NOT NULL,
  PRIMARY KEY(store_id, revision)
);
CREATE TABLE IF NOT EXISTS harness_session_event_proof_current_v1 (
  store_id TEXT PRIMARY KEY,
  revision INTEGER NOT NULL CHECK(revision > 0),
  digest TEXT NOT NULL,
  FOREIGN KEY(store_id, revision)
    REFERENCES harness_session_event_proof_history_v1(store_id, revision)
);
CREATE INDEX IF NOT EXISTS harness_session_history_exact_v4
  ON harness_session_history_v4(scope_digest, run_id, session_id, revision, digest);
CREATE INDEX IF NOT EXISTS harness_event_exact_v1
  ON harness_event_candidate_v1(source_component_id, source_epoch, source_sequence, event_digest);
`

package sqlite

const schemaV1 = `
CREATE TABLE IF NOT EXISTS model_invoker_schema (
  version INTEGER PRIMARY KEY,
  digest TEXT NOT NULL,
  applied_unix_nano INTEGER NOT NULL CHECK(applied_unix_nano > 0)
);
CREATE TABLE IF NOT EXISTS governed_model_invocation_history (
  invocation_id TEXT NOT NULL,
  revision INTEGER NOT NULL CHECK(revision > 0),
  fact_digest TEXT NOT NULL,
  attempt_digest TEXT NOT NULL,
  row_digest TEXT NOT NULL,
  canonical_json BLOB NOT NULL,
  PRIMARY KEY(invocation_id, revision)
);
CREATE TABLE IF NOT EXISTS governed_model_invocation_current (
  invocation_id TEXT PRIMARY KEY,
  revision INTEGER NOT NULL CHECK(revision > 0),
  fact_digest TEXT NOT NULL,
  highest_revision INTEGER NOT NULL CHECK(highest_revision > 0),
  FOREIGN KEY(invocation_id, revision)
    REFERENCES governed_model_invocation_history(invocation_id, revision)
);
CREATE TABLE IF NOT EXISTS governed_model_invocation_attempt_guard (
  attempt_digest TEXT PRIMARY KEY,
  invocation_id TEXT NOT NULL UNIQUE,
  FOREIGN KEY(invocation_id)
    REFERENCES governed_model_invocation_current(invocation_id)
);
CREATE INDEX IF NOT EXISTS governed_model_invocation_current_exact
  ON governed_model_invocation_current(invocation_id, revision, fact_digest, highest_revision);
CREATE INDEX IF NOT EXISTS governed_model_invocation_history_exact
  ON governed_model_invocation_history(invocation_id, revision, fact_digest, attempt_digest);
`

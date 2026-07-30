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

const schemaV2 = `
CREATE TABLE IF NOT EXISTS prepared_model_invocation_history (
  prepared_id TEXT NOT NULL,
  revision INTEGER NOT NULL CHECK(revision > 0),
  fact_digest TEXT NOT NULL,
  canonical_json BLOB NOT NULL,
  PRIMARY KEY(prepared_id, revision)
);
CREATE TABLE IF NOT EXISTS prepared_model_invocation_current (
  current_id TEXT PRIMARY KEY,
  revision INTEGER NOT NULL CHECK(revision > 0),
  current_digest TEXT NOT NULL,
  prepared_id TEXT NOT NULL,
  prepared_revision INTEGER NOT NULL CHECK(prepared_revision > 0),
  prepared_digest TEXT NOT NULL,
  canonical_json BLOB NOT NULL,
  FOREIGN KEY(prepared_id, prepared_revision)
    REFERENCES prepared_model_invocation_history(prepared_id, revision)
);
CREATE TABLE IF NOT EXISTS invocation_material_history (
  material_id TEXT NOT NULL,
  revision INTEGER NOT NULL CHECK(revision > 0),
  material_digest TEXT NOT NULL,
  route_call_digest TEXT NOT NULL,
  canonical_json BLOB NOT NULL,
  PRIMARY KEY(material_id, revision)
);
CREATE TABLE IF NOT EXISTS governed_model_turn_history (
  turn_id TEXT NOT NULL,
  revision INTEGER NOT NULL CHECK(revision > 0),
  fact_digest TEXT NOT NULL,
  attempt_digest TEXT NOT NULL,
  canonical_json BLOB NOT NULL,
  PRIMARY KEY(turn_id, revision)
);
CREATE TABLE IF NOT EXISTS governed_model_turn_current (
  turn_id TEXT PRIMARY KEY,
  revision INTEGER NOT NULL CHECK(revision > 0),
  fact_digest TEXT NOT NULL,
  highest_revision INTEGER NOT NULL CHECK(highest_revision > 0),
  FOREIGN KEY(turn_id, revision)
    REFERENCES governed_model_turn_history(turn_id, revision)
);
CREATE TABLE IF NOT EXISTS governed_model_turn_attempt_guard (
  attempt_digest TEXT PRIMARY KEY,
  turn_id TEXT NOT NULL UNIQUE,
  FOREIGN KEY(turn_id) REFERENCES governed_model_turn_current(turn_id)
);
CREATE TABLE IF NOT EXISTS governed_model_turn_tool_call_projection (
  turn_id TEXT PRIMARY KEY,
  turn_revision INTEGER NOT NULL CHECK(turn_revision > 0),
  projection_id TEXT NOT NULL UNIQUE,
  projection_revision INTEGER NOT NULL CHECK(projection_revision > 0),
  projection_digest TEXT NOT NULL,
  observation_digest TEXT NOT NULL,
  canonical_json BLOB NOT NULL,
  FOREIGN KEY(turn_id, turn_revision)
    REFERENCES governed_model_turn_history(turn_id, revision)
);
CREATE INDEX IF NOT EXISTS prepared_model_invocation_history_exact
  ON prepared_model_invocation_history(prepared_id, revision, fact_digest);
CREATE INDEX IF NOT EXISTS prepared_model_invocation_current_exact
  ON prepared_model_invocation_current(current_id, revision, current_digest, prepared_id, prepared_revision, prepared_digest);
CREATE INDEX IF NOT EXISTS invocation_material_history_exact
  ON invocation_material_history(material_id, revision, material_digest, route_call_digest);
CREATE INDEX IF NOT EXISTS governed_model_turn_history_exact
  ON governed_model_turn_history(turn_id, revision, fact_digest, attempt_digest);
CREATE INDEX IF NOT EXISTS governed_model_turn_current_exact
  ON governed_model_turn_current(turn_id, revision, fact_digest, highest_revision);
CREATE INDEX IF NOT EXISTS governed_model_turn_projection_exact
  ON governed_model_turn_tool_call_projection(projection_id, projection_revision, projection_digest, observation_digest);
`

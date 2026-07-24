package sqlite

const schemaBaseV3 = `
CREATE TABLE IF NOT EXISTS agent_host_schema (
  version INTEGER PRIMARY KEY,
  digest TEXT NOT NULL,
  applied_unix_nano INTEGER NOT NULL CHECK(applied_unix_nano > 0)
);
CREATE TABLE IF NOT EXISTS agent_host_start_claims (
  host_id TEXT NOT NULL,
  start_id TEXT NOT NULL,
  digest TEXT NOT NULL,
  row_digest TEXT NOT NULL,
  canonical_json BLOB NOT NULL,
  PRIMARY KEY(host_id, start_id)
);
CREATE TABLE IF NOT EXISTS agent_host_journal_v2 (
  host_id TEXT NOT NULL,
  start_id TEXT NOT NULL,
  revision INTEGER NOT NULL CHECK(revision > 0),
  digest TEXT NOT NULL,
  row_digest TEXT NOT NULL,
  canonical_json BLOB NOT NULL,
  PRIMARY KEY(host_id, start_id),
  FOREIGN KEY(host_id, start_id) REFERENCES agent_host_start_claims(host_id, start_id)
);
CREATE TABLE IF NOT EXISTS agent_host_cleanup_attempts_v2 (
  attempt_id TEXT PRIMARY KEY,
  host_id TEXT NOT NULL,
  start_id TEXT NOT NULL,
  revision INTEGER NOT NULL CHECK(revision > 0),
  digest TEXT NOT NULL,
  row_digest TEXT NOT NULL,
  canonical_json BLOB NOT NULL,
  FOREIGN KEY(host_id, start_id) REFERENCES agent_host_start_claims(host_id, start_id)
);
CREATE INDEX IF NOT EXISTS agent_host_cleanup_attempt_origin_v2
  ON agent_host_cleanup_attempts_v2(host_id, start_id, attempt_id);
CREATE TABLE IF NOT EXISTS agent_host_system_ready_facts_v2 (
  id TEXT PRIMARY KEY,
  revision INTEGER NOT NULL CHECK(revision = 1),
  host_id TEXT NOT NULL,
  start_id TEXT NOT NULL,
  digest TEXT NOT NULL,
  expires_unix_nano INTEGER NOT NULL CHECK(expires_unix_nano > 0),
  row_digest TEXT NOT NULL,
  canonical_json BLOB NOT NULL,
  FOREIGN KEY(host_id, start_id) REFERENCES agent_host_start_claims(host_id, start_id)
);
CREATE TABLE IF NOT EXISTS agent_host_system_ready_attempts_v2 (
  host_id TEXT NOT NULL,
  start_id TEXT NOT NULL,
  attempt_id TEXT NOT NULL,
  revision INTEGER NOT NULL CHECK(revision > 0),
  digest TEXT NOT NULL,
  row_digest TEXT NOT NULL,
  canonical_json BLOB NOT NULL,
  PRIMARY KEY(host_id, start_id, attempt_id),
  FOREIGN KEY(host_id, start_id) REFERENCES agent_host_start_claims(host_id, start_id)
);
CREATE TABLE IF NOT EXISTS agent_host_system_ready_current_history_v2 (
  id TEXT NOT NULL,
  revision INTEGER NOT NULL CHECK(revision > 0),
  epoch INTEGER NOT NULL CHECK(epoch > 0),
  fact_id TEXT NOT NULL,
  digest TEXT NOT NULL,
  expires_unix_nano INTEGER NOT NULL CHECK(expires_unix_nano > 0),
  row_digest TEXT NOT NULL,
  canonical_json BLOB NOT NULL,
  PRIMARY KEY(id, revision),
  FOREIGN KEY(fact_id) REFERENCES agent_host_system_ready_facts_v2(id)
);
CREATE TABLE IF NOT EXISTS agent_host_system_ready_current_v2 (
  id TEXT PRIMARY KEY,
  revision INTEGER NOT NULL CHECK(revision > 0),
  epoch INTEGER NOT NULL CHECK(epoch > 0),
  digest TEXT NOT NULL,
  FOREIGN KEY(id, revision) REFERENCES agent_host_system_ready_current_history_v2(id, revision)
);
CREATE INDEX IF NOT EXISTS agent_host_system_ready_current_exact_v2
  ON agent_host_system_ready_current_v2(id, revision, epoch, digest);
`

const schemaDeltaV4 = `
CREATE TABLE IF NOT EXISTS agent_host_review_model_association_history_v1 (
  id TEXT NOT NULL,
  revision INTEGER NOT NULL CHECK(revision > 0),
  digest TEXT NOT NULL,
  previous_digest TEXT NOT NULL,
  checked_unix_nano INTEGER NOT NULL CHECK(checked_unix_nano > 0),
  expires_unix_nano INTEGER NOT NULL CHECK(expires_unix_nano > checked_unix_nano),
  row_digest TEXT NOT NULL,
  canonical_json BLOB NOT NULL,
  PRIMARY KEY(id, revision),
  UNIQUE(id, revision, digest)
);
CREATE TABLE IF NOT EXISTS agent_host_review_model_association_current_v1 (
  id TEXT PRIMARY KEY,
  revision INTEGER NOT NULL,
  digest TEXT NOT NULL,
  FOREIGN KEY(id, revision, digest) REFERENCES agent_host_review_model_association_history_v1(id, revision, digest)
);
`

const schemaV1 = schemaBaseV3 + schemaDeltaV4

const schemaDeltaV5 = `
CREATE TABLE IF NOT EXISTS agent_host_start_claim_input_bindings_v3 (
  host_id TEXT NOT NULL,
  start_id TEXT NOT NULL,
  claim_digest TEXT NOT NULL,
  input_digest TEXT NOT NULL,
  binding_digest TEXT NOT NULL,
  row_digest TEXT NOT NULL,
  canonical_json BLOB NOT NULL,
  PRIMARY KEY(host_id, start_id),
  FOREIGN KEY(host_id, start_id) REFERENCES agent_host_start_claims(host_id, start_id)
);
CREATE UNIQUE INDEX IF NOT EXISTS agent_host_start_claim_input_exact_v3
  ON agent_host_start_claim_input_bindings_v3(host_id,start_id,claim_digest,input_digest,binding_digest);
`

const schemaV5 = schemaV1 + schemaDeltaV5

const schemaDeltaV6 = `
CREATE TABLE IF NOT EXISTS agent_host_cleanup_closures_v2 (
  closure_id TEXT PRIMARY KEY,
  host_id TEXT NOT NULL,
  start_id TEXT NOT NULL,
  revision INTEGER NOT NULL CHECK(revision = 1),
  digest TEXT NOT NULL,
  plan_id TEXT NOT NULL,
  plan_revision INTEGER NOT NULL CHECK(plan_revision > 0),
  plan_digest TEXT NOT NULL,
  coverage_digest TEXT NOT NULL,
  row_digest TEXT NOT NULL,
  canonical_json BLOB NOT NULL,
  UNIQUE(host_id, start_id),
  FOREIGN KEY(host_id, start_id) REFERENCES agent_host_start_claims(host_id, start_id)
);
CREATE INDEX IF NOT EXISTS agent_host_cleanup_closure_plan_v2
  ON agent_host_cleanup_closures_v2(plan_id, plan_revision, plan_digest);
`

const schemaV6 = schemaV5 + schemaDeltaV6

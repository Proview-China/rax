package reviewcontextstore

const reviewerContextSQLiteSchemaV1 = `
CREATE TABLE IF NOT EXISTS context_reviewer_context_schema (
  version INTEGER PRIMARY KEY,
  digest TEXT NOT NULL
);
CREATE TABLE IF NOT EXISTS context_reviewer_context_history (
  tenant_id TEXT NOT NULL,
  envelope_id TEXT NOT NULL,
  revision INTEGER NOT NULL CHECK(revision > 0),
  envelope_digest TEXT NOT NULL,
  row_digest TEXT NOT NULL,
  payload BLOB NOT NULL,
  PRIMARY KEY(tenant_id, envelope_id, revision)
);
CREATE TABLE IF NOT EXISTS context_reviewer_context_current (
  tenant_id TEXT NOT NULL,
  envelope_id TEXT NOT NULL,
  revision INTEGER NOT NULL CHECK(revision > 0),
  envelope_digest TEXT NOT NULL,
  highest_revision INTEGER NOT NULL CHECK(highest_revision > 0),
  PRIMARY KEY(tenant_id, envelope_id),
  FOREIGN KEY(tenant_id, envelope_id, revision)
    REFERENCES context_reviewer_context_history(tenant_id, envelope_id, revision)
);
CREATE INDEX IF NOT EXISTS context_reviewer_context_current_exact
  ON context_reviewer_context_current(tenant_id, envelope_id, revision, envelope_digest, highest_revision);
`

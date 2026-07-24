package assemblypublication

const sqlitePublicationSchemaVersionV2 = 1

const sqlitePublicationSchemaV2 = `
CREATE TABLE IF NOT EXISTS harness_publication_schema_v2 (
  version INTEGER PRIMARY KEY,
  digest TEXT NOT NULL,
  applied_unix_nano INTEGER NOT NULL CHECK(applied_unix_nano > 0)
);
CREATE TABLE IF NOT EXISTS harness_publication_staged_v2 (
  publication_id TEXT PRIMARY KEY,
  generation_digest TEXT,
  generation_row_digest TEXT,
  generation_json BLOB,
  manifest_digest TEXT,
  manifest_row_digest TEXT,
  manifest_json BLOB,
  graph_digest TEXT,
  graph_row_digest TEXT,
  graph_json BLOB,
  handoff_digest TEXT,
  handoff_row_digest TEXT,
  handoff_json BLOB
);
CREATE TABLE IF NOT EXISTS harness_publication_committed_v2 (
  publication_id TEXT PRIMARY KEY,
  revision INTEGER NOT NULL CHECK(revision = 1),
  digest TEXT NOT NULL,
  scope_ref TEXT NOT NULL,
  bundle_row_digest TEXT NOT NULL,
  bundle_json BLOB NOT NULL,
  current_row_digest TEXT NOT NULL,
  current_json BLOB NOT NULL
);
CREATE TABLE IF NOT EXISTS harness_publication_current_history_v2 (
  scope_ref TEXT NOT NULL,
  revision INTEGER NOT NULL CHECK(revision > 0),
  digest TEXT NOT NULL,
  publication_id TEXT NOT NULL UNIQUE,
  row_digest TEXT NOT NULL,
  canonical_json BLOB NOT NULL,
  PRIMARY KEY(scope_ref, revision),
  FOREIGN KEY(publication_id) REFERENCES harness_publication_committed_v2(publication_id)
);
CREATE TABLE IF NOT EXISTS harness_publication_current_v2 (
  scope_ref TEXT PRIMARY KEY,
  revision INTEGER NOT NULL CHECK(revision > 0),
  digest TEXT NOT NULL,
  publication_id TEXT NOT NULL,
  FOREIGN KEY(scope_ref, revision) REFERENCES harness_publication_current_history_v2(scope_ref, revision),
  FOREIGN KEY(publication_id) REFERENCES harness_publication_committed_v2(publication_id)
);
CREATE INDEX IF NOT EXISTS harness_publication_current_exact_v2
  ON harness_publication_current_v2(scope_ref, revision, digest, publication_id);
`

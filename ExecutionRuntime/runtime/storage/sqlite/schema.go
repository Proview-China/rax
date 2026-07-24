package sqlite

const schemaV1 = `
CREATE TABLE IF NOT EXISTS runtime_binding_schema (
  version INTEGER PRIMARY KEY,
  digest TEXT NOT NULL,
  applied_unix_nano INTEGER NOT NULL CHECK(applied_unix_nano > 0)
);
CREATE TABLE IF NOT EXISTS runtime_binding_facts (
  id TEXT PRIMARY KEY,
  revision INTEGER NOT NULL CHECK(revision > 0),
  digest TEXT NOT NULL,
  canonical_json BLOB NOT NULL
);
CREATE TABLE IF NOT EXISTS runtime_binding_sets (
  id TEXT PRIMARY KEY,
  revision INTEGER NOT NULL CHECK(revision > 0),
  digest TEXT NOT NULL,
  canonical_json BLOB NOT NULL
);
CREATE TABLE IF NOT EXISTS runtime_review_binding_association_history (
  id TEXT NOT NULL,
  revision INTEGER NOT NULL CHECK(revision > 0),
  digest TEXT NOT NULL,
  canonical_json BLOB NOT NULL,
  PRIMARY KEY(id, revision)
);
CREATE TABLE IF NOT EXISTS runtime_review_binding_association_current (
  id TEXT PRIMARY KEY,
  revision INTEGER NOT NULL CHECK(revision > 0),
  digest TEXT NOT NULL,
  FOREIGN KEY(id, revision) REFERENCES runtime_review_binding_association_history(id, revision)
);
CREATE TABLE IF NOT EXISTS runtime_review_binding_projection_history (
  tenant_id TEXT NOT NULL,
  id TEXT NOT NULL,
  revision INTEGER NOT NULL CHECK(revision > 0),
  digest TEXT NOT NULL,
  canonical_json BLOB NOT NULL,
  PRIMARY KEY(tenant_id, id, revision)
);
CREATE TABLE IF NOT EXISTS runtime_review_binding_projection_current (
  tenant_id TEXT NOT NULL,
  id TEXT NOT NULL,
  revision INTEGER NOT NULL CHECK(revision > 0),
  digest TEXT NOT NULL,
  highest_revision INTEGER NOT NULL CHECK(highest_revision > 0),
  PRIMARY KEY(tenant_id, id),
  FOREIGN KEY(tenant_id, id, revision) REFERENCES runtime_review_binding_projection_history(tenant_id, id, revision)
);
CREATE TABLE IF NOT EXISTS runtime_review_binding_publish_receipts (
  publish_id TEXT PRIMARY KEY,
  tenant_id TEXT NOT NULL,
  digest TEXT NOT NULL,
  canonical_json BLOB NOT NULL
);
CREATE INDEX IF NOT EXISTS runtime_review_binding_projection_identity
  ON runtime_review_binding_projection_current(tenant_id, id, revision, digest);
CREATE TABLE IF NOT EXISTS runtime_evidence_subject_projection_history (
  projection_id TEXT NOT NULL,
  revision INTEGER NOT NULL CHECK(revision > 0),
  subject_digest TEXT NOT NULL,
  digest TEXT NOT NULL,
  canonical_json BLOB NOT NULL,
  PRIMARY KEY(projection_id, revision)
);
CREATE TABLE IF NOT EXISTS runtime_evidence_subject_current (
  subject_digest TEXT PRIMARY KEY,
  index_id TEXT NOT NULL,
  revision INTEGER NOT NULL CHECK(revision > 0),
  projection_id TEXT NOT NULL,
  projection_revision INTEGER NOT NULL CHECK(projection_revision > 0),
  projection_digest TEXT NOT NULL,
  owner_watermark INTEGER NOT NULL CHECK(owner_watermark > 0),
  digest TEXT NOT NULL,
  canonical_json BLOB NOT NULL,
  FOREIGN KEY(projection_id, projection_revision) REFERENCES runtime_evidence_subject_projection_history(projection_id, revision)
);
CREATE TABLE IF NOT EXISTS runtime_evidence_subject_mutation_commits (
  mutation_id TEXT PRIMARY KEY,
  stable_key_digest TEXT NOT NULL,
  request_digest TEXT NOT NULL,
  subject_digest TEXT NOT NULL,
  digest TEXT NOT NULL,
  canonical_json BLOB NOT NULL
);
CREATE INDEX IF NOT EXISTS runtime_evidence_subject_current_identity
  ON runtime_evidence_subject_current(subject_digest, index_id, revision, projection_id, projection_revision, projection_digest);
CREATE TABLE IF NOT EXISTS runtime_review_evidence_projection_history (
  projection_id TEXT NOT NULL,
  revision INTEGER NOT NULL CHECK(revision > 0),
  subject_digest TEXT NOT NULL,
  digest TEXT NOT NULL,
  canonical_json BLOB NOT NULL,
  PRIMARY KEY(projection_id, revision)
);
CREATE TABLE IF NOT EXISTS runtime_review_evidence_current (
  subject_digest TEXT PRIMARY KEY,
  index_id TEXT NOT NULL,
  revision INTEGER NOT NULL CHECK(revision > 0),
  projection_id TEXT NOT NULL,
  projection_revision INTEGER NOT NULL CHECK(projection_revision > 0),
  projection_digest TEXT NOT NULL,
  highest_revision INTEGER NOT NULL CHECK(highest_revision > 0),
  digest TEXT NOT NULL,
  canonical_json BLOB NOT NULL,
  FOREIGN KEY(projection_id, projection_revision) REFERENCES runtime_review_evidence_projection_history(projection_id, revision)
);
CREATE TABLE IF NOT EXISTS runtime_review_evidence_publish_receipts (
  publish_id TEXT PRIMARY KEY,
  request_digest TEXT NOT NULL,
  subject_digest TEXT NOT NULL,
  projection_id TEXT NOT NULL,
  projection_revision INTEGER NOT NULL CHECK(projection_revision > 0),
  projection_digest TEXT NOT NULL,
  digest TEXT NOT NULL,
  canonical_json BLOB NOT NULL
);
CREATE INDEX IF NOT EXISTS runtime_review_evidence_current_identity
  ON runtime_review_evidence_current(subject_digest, index_id, revision, projection_id, projection_revision, projection_digest, highest_revision);
`

const schemaV2 = `
CREATE TABLE IF NOT EXISTS runtime_review_governance_source_history (
  kind TEXT NOT NULL,
  source_ref TEXT NOT NULL,
  revision INTEGER NOT NULL CHECK(revision > 0),
  fact_digest TEXT NOT NULL,
  row_digest TEXT NOT NULL,
  canonical_json BLOB NOT NULL,
  PRIMARY KEY(kind, source_ref, revision)
);
CREATE TABLE IF NOT EXISTS runtime_review_governance_source_current (
  kind TEXT NOT NULL,
  source_ref TEXT NOT NULL,
  revision INTEGER NOT NULL CHECK(revision > 0),
  fact_digest TEXT NOT NULL,
  PRIMARY KEY(kind, source_ref),
  FOREIGN KEY(kind, source_ref, revision) REFERENCES runtime_review_governance_source_history(kind, source_ref, revision)
);
CREATE INDEX IF NOT EXISTS runtime_review_governance_source_identity
  ON runtime_review_governance_source_current(kind, source_ref, revision, fact_digest);
CREATE TABLE IF NOT EXISTS runtime_review_governance_projection_history (
  kind TEXT NOT NULL,
  tenant_id TEXT NOT NULL,
  projection_id TEXT NOT NULL,
  revision INTEGER NOT NULL CHECK(revision > 0),
  projection_digest TEXT NOT NULL,
  row_digest TEXT NOT NULL,
  canonical_json BLOB NOT NULL,
  PRIMARY KEY(kind, tenant_id, projection_id, revision)
);
CREATE TABLE IF NOT EXISTS runtime_review_governance_projection_current (
  kind TEXT NOT NULL,
  tenant_id TEXT NOT NULL,
  projection_id TEXT NOT NULL,
  revision INTEGER NOT NULL CHECK(revision > 0),
  projection_digest TEXT NOT NULL,
  highest_revision INTEGER NOT NULL CHECK(highest_revision > 0),
  PRIMARY KEY(kind, tenant_id, projection_id),
  FOREIGN KEY(kind, tenant_id, projection_id, revision) REFERENCES runtime_review_governance_projection_history(kind, tenant_id, projection_id, revision)
);
CREATE INDEX IF NOT EXISTS runtime_review_governance_projection_identity
  ON runtime_review_governance_projection_current(kind, tenant_id, projection_id, revision, projection_digest, highest_revision);
CREATE UNIQUE INDEX IF NOT EXISTS runtime_review_governance_projection_global_ref
  ON runtime_review_governance_projection_history(kind, projection_id, revision);
`

const schemaV3 = `
CREATE TABLE IF NOT EXISTS runtime_operation_review_authorization_history (
  contract_version INTEGER NOT NULL CHECK(contract_version IN (4,5)),
  tenant_id TEXT NOT NULL,
  authorization_id TEXT NOT NULL,
  revision INTEGER NOT NULL CHECK(revision > 0),
  fact_digest TEXT NOT NULL,
  operation_digest TEXT NOT NULL,
  effect_id TEXT NOT NULL,
  state TEXT NOT NULL,
  row_digest TEXT NOT NULL,
  canonical_json BLOB NOT NULL,
  PRIMARY KEY(contract_version, authorization_id, revision)
);
CREATE TABLE IF NOT EXISTS runtime_operation_review_authorization_current (
  contract_version INTEGER NOT NULL CHECK(contract_version IN (4,5)),
  tenant_id TEXT NOT NULL,
  authorization_id TEXT NOT NULL,
  revision INTEGER NOT NULL CHECK(revision > 0),
  fact_digest TEXT NOT NULL,
  highest_revision INTEGER NOT NULL CHECK(highest_revision > 0),
  PRIMARY KEY(contract_version, authorization_id),
  FOREIGN KEY(contract_version, authorization_id, revision)
    REFERENCES runtime_operation_review_authorization_history(contract_version, authorization_id, revision)
);
CREATE INDEX IF NOT EXISTS runtime_operation_review_authorization_current_exact
  ON runtime_operation_review_authorization_current(contract_version, tenant_id, authorization_id, revision, fact_digest, highest_revision);
CREATE TABLE IF NOT EXISTS runtime_operation_review_authorization_active_guard (
  tenant_id TEXT NOT NULL,
  operation_digest TEXT NOT NULL,
  effect_id TEXT NOT NULL,
  contract_version INTEGER NOT NULL CHECK(contract_version IN (4,5)),
  authorization_id TEXT NOT NULL,
  authorization_revision INTEGER NOT NULL CHECK(authorization_revision > 0),
  authorization_digest TEXT NOT NULL,
  PRIMARY KEY(tenant_id, operation_digest, effect_id),
  UNIQUE(contract_version, authorization_id),
  FOREIGN KEY(contract_version, authorization_id, authorization_revision)
    REFERENCES runtime_operation_review_authorization_history(contract_version, authorization_id, revision)
);
CREATE INDEX IF NOT EXISTS runtime_operation_review_authorization_history_subject
  ON runtime_operation_review_authorization_history(tenant_id, operation_digest, effect_id, contract_version, authorization_id, revision);
`

const schemaV4 = `
CREATE TABLE IF NOT EXISTS runtime_binding_admission_attempts (
  attempt_id TEXT PRIMARY KEY,
  revision INTEGER NOT NULL CHECK(revision > 0),
  digest TEXT NOT NULL,
  canonical_json BLOB NOT NULL
);
CREATE INDEX IF NOT EXISTS runtime_binding_admission_attempt_exact
  ON runtime_binding_admission_attempts(attempt_id, revision, digest);
`

const schemaV5 = `
CREATE TABLE IF NOT EXISTS runtime_generation_binding_association_history (
  association_id TEXT NOT NULL,
  revision INTEGER NOT NULL CHECK(revision > 0),
  fact_digest TEXT NOT NULL,
  row_digest TEXT NOT NULL,
  canonical_json BLOB NOT NULL,
  PRIMARY KEY(association_id, revision)
);
CREATE TABLE IF NOT EXISTS runtime_generation_binding_association_current (
  association_id TEXT PRIMARY KEY,
  revision INTEGER NOT NULL CHECK(revision > 0),
  fact_digest TEXT NOT NULL,
  highest_revision INTEGER NOT NULL CHECK(highest_revision > 0),
  FOREIGN KEY(association_id, revision)
    REFERENCES runtime_generation_binding_association_history(association_id, revision)
);
CREATE INDEX IF NOT EXISTS runtime_generation_binding_association_current_exact
  ON runtime_generation_binding_association_current(association_id, revision, fact_digest, highest_revision);
`

const schemaV6 = `
CREATE TABLE IF NOT EXISTS runtime_resource_handle_history (
  owner_domain TEXT NOT NULL,
  owner_id TEXT NOT NULL,
  handle_id TEXT NOT NULL,
  revision INTEGER NOT NULL CHECK(revision > 0),
  projection_digest TEXT NOT NULL,
  row_digest TEXT NOT NULL,
  canonical_json BLOB NOT NULL,
  PRIMARY KEY(owner_domain, owner_id, handle_id, revision)
);
CREATE TABLE IF NOT EXISTS runtime_resource_handle_current (
  owner_domain TEXT NOT NULL,
  owner_id TEXT NOT NULL,
  handle_id TEXT NOT NULL,
  revision INTEGER NOT NULL CHECK(revision > 0),
  projection_digest TEXT NOT NULL,
  highest_revision INTEGER NOT NULL CHECK(highest_revision > 0),
  PRIMARY KEY(owner_domain, owner_id, handle_id),
  FOREIGN KEY(owner_domain, owner_id, handle_id, revision)
    REFERENCES runtime_resource_handle_history(owner_domain, owner_id, handle_id, revision)
);
CREATE TABLE IF NOT EXISTS runtime_resource_binding_set_history (
  set_id TEXT NOT NULL,
  revision INTEGER NOT NULL CHECK(revision > 0),
  projection_digest TEXT NOT NULL,
  row_digest TEXT NOT NULL,
  canonical_json BLOB NOT NULL,
  PRIMARY KEY(set_id, revision)
);
CREATE TABLE IF NOT EXISTS runtime_resource_binding_set_current (
  set_id TEXT PRIMARY KEY,
  revision INTEGER NOT NULL CHECK(revision > 0),
  projection_digest TEXT NOT NULL,
  highest_revision INTEGER NOT NULL CHECK(highest_revision > 0),
  FOREIGN KEY(set_id, revision)
    REFERENCES runtime_resource_binding_set_history(set_id, revision)
);
CREATE INDEX IF NOT EXISTS runtime_resource_handle_current_exact
  ON runtime_resource_handle_current(owner_domain, owner_id, handle_id, revision, projection_digest, highest_revision);
CREATE INDEX IF NOT EXISTS runtime_resource_binding_set_current_exact
  ON runtime_resource_binding_set_current(set_id, revision, projection_digest, highest_revision);
`

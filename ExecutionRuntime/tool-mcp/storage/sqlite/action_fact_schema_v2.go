package sqlite

import "github.com/Proview-China/rax/ExecutionRuntime/runtime/core"

const actionFactSchemaVersionV2 = 2

const actionFactSchemaV2 = `
CREATE TABLE IF NOT EXISTS tool_action_fact_schema_v2 (
    version INTEGER PRIMARY KEY,
    digest TEXT NOT NULL,
    applied_unix_nano INTEGER NOT NULL
) STRICT;

CREATE TABLE IF NOT EXISTS tool_action_candidate_v2 (
    fact_id TEXT PRIMARY KEY,
    revision INTEGER NOT NULL,
    digest TEXT NOT NULL,
    action_id TEXT NOT NULL UNIQUE,
    body_json BLOB NOT NULL,
    row_digest TEXT NOT NULL,
    UNIQUE(fact_id,revision,digest)
) STRICT;

CREATE TABLE IF NOT EXISTS tool_action_reservation_v2 (
    fact_id TEXT PRIMARY KEY,
    revision INTEGER NOT NULL,
    digest TEXT NOT NULL,
    action_id TEXT NOT NULL UNIQUE,
    body_json BLOB NOT NULL,
    row_digest TEXT NOT NULL,
    UNIQUE(fact_id,revision,digest)
) STRICT;

CREATE TABLE IF NOT EXISTS tool_domain_result_v2 (
    fact_id TEXT PRIMARY KEY,
    revision INTEGER NOT NULL,
    digest TEXT NOT NULL,
    action_id TEXT NOT NULL UNIQUE,
    body_json BLOB NOT NULL,
    row_digest TEXT NOT NULL,
    UNIQUE(fact_id,revision,digest)
) STRICT;

CREATE TABLE IF NOT EXISTS tool_apply_settlement_v2 (
    fact_id TEXT PRIMARY KEY,
    revision INTEGER NOT NULL,
    digest TEXT NOT NULL,
    action_id TEXT NOT NULL UNIQUE,
    body_json BLOB NOT NULL,
    row_digest TEXT NOT NULL,
    UNIQUE(fact_id,revision,digest)
) STRICT;

CREATE TABLE IF NOT EXISTS tool_result_v2 (
    fact_id TEXT PRIMARY KEY,
    revision INTEGER NOT NULL,
    digest TEXT NOT NULL,
    action_id TEXT NOT NULL UNIQUE,
    apply_id TEXT NOT NULL UNIQUE,
    body_json BLOB NOT NULL,
    row_digest TEXT NOT NULL,
    UNIQUE(fact_id,revision,digest)
) STRICT;

CREATE TABLE IF NOT EXISTS tool_action_head_v2 (
    action_id TEXT PRIMARY KEY,
    head_revision INTEGER NOT NULL,
    head_digest TEXT NOT NULL,
    stage TEXT NOT NULL CHECK(stage IN ('candidate','reserved','domain_result','settled')),
    candidate_id TEXT NOT NULL,
    candidate_revision INTEGER NOT NULL,
    candidate_digest TEXT NOT NULL,
    reservation_id TEXT,
    reservation_revision INTEGER,
    reservation_digest TEXT,
    domain_result_id TEXT,
    domain_result_revision INTEGER,
    domain_result_digest TEXT,
    apply_id TEXT,
    apply_revision INTEGER,
    apply_digest TEXT,
    result_id TEXT,
    result_revision INTEGER,
    result_digest TEXT,
    updated_unix_nano INTEGER NOT NULL,
    row_digest TEXT NOT NULL
) STRICT;
`

var actionFactSchemaDigestV2 = core.DigestBytes([]byte(actionFactSchemaV2))

var actionFactOwnedObjectsV2 = []string{
	"tool_action_fact_schema_v2",
	"tool_action_candidate_v2",
	"tool_action_reservation_v2",
	"tool_domain_result_v2",
	"tool_apply_settlement_v2",
	"tool_result_v2",
	"tool_action_head_v2",
}

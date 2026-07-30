package sqlite

import (
	"context"
	"database/sql"
)

func verifyHostStartPackageSelectionSchemaV1(ctx context.Context, tx *sql.Tx) error {
	return verifyDeploymentTableV2(ctx, tx, deploymentSchemaTableV2{
		Name:   "agent_host_start_package_selection_bindings_v1",
		Strict: 1,
		Columns: []deploymentSchemaColumnV2{
			{Name: "host_id", Type: "TEXT", NotNull: 1, PrimaryKey: 1},
			{Name: "start_id", Type: "TEXT", NotNull: 1, PrimaryKey: 2},
			{Name: "claim_digest", Type: "TEXT", NotNull: 1},
			{Name: "claim_input_binding_digest", Type: "TEXT", NotNull: 1},
			{Name: "deployment_id", Type: "TEXT", NotNull: 1},
			{Name: "deployment_revision", Type: "INTEGER", NotNull: 1},
			{Name: "deployment_digest", Type: "TEXT", NotNull: 1},
			{Name: "deployment_expires_unix_nano", Type: "INTEGER", NotNull: 1},
			{Name: "selection_id", Type: "TEXT", NotNull: 1},
			{Name: "selection_revision", Type: "INTEGER", NotNull: 1},
			{Name: "selection_digest", Type: "TEXT", NotNull: 1},
			{Name: "selection_expires_unix_nano", Type: "INTEGER", NotNull: 1},
			{Name: "closure_digest", Type: "TEXT", NotNull: 1},
			{Name: "revision", Type: "INTEGER", NotNull: 1},
			{Name: "created_unix_nano", Type: "INTEGER", NotNull: 1},
			{Name: "expires_unix_nano", Type: "INTEGER", NotNull: 1},
			{Name: "binding_digest", Type: "TEXT", NotNull: 1},
			{Name: "row_digest", Type: "TEXT", NotNull: 1},
			{Name: "canonical_json", Type: "BLOB", NotNull: 1},
		},
		PrimaryIndex: []string{"host_id", "start_id"},
		ForeignKeys: []deploymentSchemaForeignKeyV2{
			{Sequence: 0, Table: "agent_host_deployment_current_history_v2", From: "host_id", To: "host_id", OnUpdate: "NO ACTION", OnDelete: "NO ACTION", Match: "NONE"},
			{Sequence: 1, Table: "agent_host_deployment_current_history_v2", From: "deployment_id", To: "deployment_id", OnUpdate: "NO ACTION", OnDelete: "NO ACTION", Match: "NONE"},
			{Sequence: 2, Table: "agent_host_deployment_current_history_v2", From: "deployment_revision", To: "revision", OnUpdate: "NO ACTION", OnDelete: "NO ACTION", Match: "NONE"},
			{Sequence: 3, Table: "agent_host_deployment_current_history_v2", From: "deployment_digest", To: "digest", OnUpdate: "NO ACTION", OnDelete: "NO ACTION", Match: "NONE"},
			{Sequence: 4, Table: "agent_host_deployment_current_history_v2", From: "selection_id", To: "selection_id", OnUpdate: "NO ACTION", OnDelete: "NO ACTION", Match: "NONE"},
			{Sequence: 5, Table: "agent_host_deployment_current_history_v2", From: "selection_revision", To: "selection_revision", OnUpdate: "NO ACTION", OnDelete: "NO ACTION", Match: "NONE"},
			{Sequence: 6, Table: "agent_host_deployment_current_history_v2", From: "selection_digest", To: "selection_digest", OnUpdate: "NO ACTION", OnDelete: "NO ACTION", Match: "NONE"},
			{Sequence: 7, Table: "agent_host_deployment_current_history_v2", From: "selection_expires_unix_nano", To: "selection_expires_unix_nano", OnUpdate: "NO ACTION", OnDelete: "NO ACTION", Match: "NONE"},
		},
		ExpectedDDL: `CREATE TABLE agent_host_start_package_selection_bindings_v1 (
		  host_id TEXT NOT NULL,
		  start_id TEXT NOT NULL,
		  claim_digest TEXT NOT NULL,
		  claim_input_binding_digest TEXT NOT NULL,
		  deployment_id TEXT NOT NULL,
		  deployment_revision INTEGER NOT NULL CHECK(deployment_revision > 0),
		  deployment_digest TEXT NOT NULL,
		  deployment_expires_unix_nano INTEGER NOT NULL CHECK(deployment_expires_unix_nano > 0),
		  selection_id TEXT NOT NULL,
		  selection_revision INTEGER NOT NULL CHECK(selection_revision > 0),
		  selection_digest TEXT NOT NULL,
		  selection_expires_unix_nano INTEGER NOT NULL CHECK(selection_expires_unix_nano > 0),
		  closure_digest TEXT NOT NULL,
		  revision INTEGER NOT NULL CHECK(revision = 1),
		  created_unix_nano INTEGER NOT NULL CHECK(created_unix_nano > 0),
		  expires_unix_nano INTEGER NOT NULL CHECK(expires_unix_nano > created_unix_nano),
		  binding_digest TEXT NOT NULL,
		  row_digest TEXT NOT NULL,
		  canonical_json BLOB NOT NULL,
		  PRIMARY KEY(host_id, start_id),
		  FOREIGN KEY(host_id, deployment_id, deployment_revision, deployment_digest, selection_id, selection_revision, selection_digest, selection_expires_unix_nano)
		    REFERENCES agent_host_deployment_current_history_v2(host_id, deployment_id, revision, digest, selection_id, selection_revision, selection_digest, selection_expires_unix_nano)
		) STRICT`,
	})
}

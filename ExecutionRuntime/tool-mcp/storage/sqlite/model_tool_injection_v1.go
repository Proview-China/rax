// Package sqlite provides Tool Owner-local single-node durable exact readers.
// WAL/FULL are crash-durability details; this package makes no HA, remote
// durability, production composition-root, provider, or SLA claim.
package sqlite

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	toolcontract "github.com/Proview-China/rax/ExecutionRuntime/tool-mcp/contract"
	toolsurface "github.com/Proview-China/rax/ExecutionRuntime/tool-mcp/surface"
	_ "modernc.org/sqlite"
)

const schemaVersionV1 = 1

const schemaV1 = `
CREATE TABLE IF NOT EXISTS tool_owner_schema_v1 (
    version INTEGER PRIMARY KEY,
    digest TEXT NOT NULL,
    applied_unix_nano INTEGER NOT NULL
) STRICT;

CREATE TABLE IF NOT EXISTS model_tool_injection_material_v1 (
    material_id TEXT PRIMARY KEY,
    revision INTEGER NOT NULL,
    digest TEXT NOT NULL,
    surface_id TEXT NOT NULL,
	    surface_revision INTEGER NOT NULL,
	    surface_digest TEXT NOT NULL,
	    compiled_tools_digest TEXT NOT NULL,
	    expires_unix_nano INTEGER NOT NULL,
	    compiled_tools_json BLOB NOT NULL,
	    body_json BLOB NOT NULL,
	    row_digest TEXT NOT NULL,
    UNIQUE(material_id, revision, digest)
) STRICT;

CREATE TABLE IF NOT EXISTS tool_surface_invocation_binding_v1 (
    binding_id TEXT PRIMARY KEY,
    revision INTEGER NOT NULL,
    digest TEXT NOT NULL,
    invocation_id TEXT NOT NULL,
    invocation_digest TEXT NOT NULL,
    expires_unix_nano INTEGER NOT NULL,
    request_digest TEXT NOT NULL,
    request_json BLOB NOT NULL,
    binding_json BLOB NOT NULL,
    ack_json BLOB NOT NULL,
    row_digest TEXT NOT NULL,
    UNIQUE(invocation_id, invocation_digest),
    UNIQUE(binding_id, revision, digest)
) STRICT;
`

type ConfigV1 struct {
	Path         string
	BusyTimeout  time.Duration
	MaxOpenConns int
	Clock        func() time.Time
	Owner        core.OwnerRef
}

type StoreV1 struct {
	db    *sql.DB
	clock func() time.Time
	owner core.OwnerRef

	gatesMu sync.Mutex
	gates   map[string]*bindingInvocationGateV1
}

type bindingInvocationGateV1 struct {
	mu   sync.Mutex
	refs int
}

func OpenV1(ctx context.Context, config ConfigV1) (*StoreV1, error) {
	if err := contextErrorV1(ctx); err != nil {
		return nil, err
	}
	if strings.TrimSpace(config.Path) == "" {
		return nil, invalidV1("Tool SQLite path is required")
	}
	if err := config.Owner.Validate(); err != nil {
		return nil, invalidV1("Tool SQLite Owner is required")
	}
	if config.BusyTimeout <= 0 {
		config.BusyTimeout = 5 * time.Second
	}
	if config.BusyTimeout > time.Minute {
		return nil, invalidV1("Tool SQLite busy timeout exceeds one minute")
	}
	if config.MaxOpenConns <= 0 {
		config.MaxOpenConns = 8
	}
	if config.MaxOpenConns > 32 {
		return nil, invalidV1("Tool SQLite connection count exceeds 32")
	}
	if config.Clock == nil {
		config.Clock = time.Now
	}
	absolute, err := filepath.Abs(config.Path)
	if err != nil {
		return nil, invalidV1("Tool SQLite path is invalid")
	}
	dsn := (&url.URL{Scheme: "file", Path: absolute}).String()
	dsn += fmt.Sprintf("?_pragma=journal_mode(WAL)&_pragma=synchronous(FULL)&_pragma=foreign_keys(1)&_pragma=busy_timeout(%d)&_txlock=immediate", config.BusyTimeout.Milliseconds())
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, mapDBErrorV1(ctx, err, false)
	}
	db.SetMaxOpenConns(config.MaxOpenConns)
	db.SetMaxIdleConns(config.MaxOpenConns)
	store := &StoreV1{db: db, clock: config.Clock, owner: config.Owner, gates: make(map[string]*bindingInvocationGateV1)}
	if err := store.migrateV1(ctx); err != nil {
		_ = db.Close()
		return nil, err
	}
	if err := store.verifyV1(ctx); err != nil {
		_ = db.Close()
		return nil, err
	}
	return store, nil
}

func (s *StoreV1) Close() error {
	if s == nil || s.db == nil {
		return nil
	}
	return s.db.Close()
}

func (s *StoreV1) IntegrityCheckV1(ctx context.Context) error {
	if err := s.readReadyV1(ctx); err != nil {
		return err
	}
	var result string
	if err := s.db.QueryRowContext(ctx, `PRAGMA integrity_check`).Scan(&result); err != nil {
		return mapDBErrorV1(ctx, err, false)
	}
	if result != "ok" {
		return conflictV1("Tool SQLite integrity check failed")
	}
	return nil
}

func (s *StoreV1) CompileAndEnsureModelToolInjectionMaterialV1(
	ctx context.Context,
	exactSurface toolcontract.ToolSurfaceManifestCurrentRefV1,
	surfaces toolcontract.ToolSurfaceManifestCurrentReaderV1,
	definitions toolcontract.ToolDefinitionMaterialReaderV1,
	registry toolcontract.ToolRegistryObjectCurrentReaderV1,
) (toolsurface.CompiledModelToolsV1, toolcontract.ModelToolInjectionMaterialV1, error) {
	if err := s.writeReadyV1(ctx); err != nil {
		return modelToolClosureZeroV1(err)
	}
	compiled, material, err := toolsurface.CompileModelToolInjectionMaterialV1(
		ctx, exactSurface, surfaces, definitions, registry, s.clock,
	)
	if err != nil {
		return modelToolClosureZeroV1(err)
	}
	return s.ensureCompiledModelToolInjectionMaterialV1(ctx, compiled, material)
}

func (s *StoreV1) ensureCompiledModelToolInjectionMaterialV1(
	ctx context.Context,
	compiled toolsurface.CompiledModelToolsV1,
	material toolcontract.ModelToolInjectionMaterialV1,
) (toolsurface.CompiledModelToolsV1, toolcontract.ModelToolInjectionMaterialV1, error) {
	compiled = compiled.Clone()
	material = material.Clone()
	now := s.clock()
	if err := material.ValidateCurrent(material.Ref, now); err != nil {
		return modelToolClosureZeroV1(err)
	}
	if err := compiled.ValidateAgainstMaterialV1(material); err != nil {
		return modelToolClosureZeroV1(err)
	}
	rowDigest, err := modelToolClosureRowDigestV1(compiled, material)
	if err != nil {
		return modelToolClosureZeroV1(err)
	}
	compiledBody, err := json.Marshal(compiled)
	if err != nil {
		return modelToolClosureZeroV1(invalidV1("Compiled Model Tools JSON encode failed"))
	}
	materialBody, err := json.Marshal(material)
	if err != nil {
		return modelToolClosureZeroV1(invalidV1("Model Tool Injection Material JSON encode failed"))
	}
	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelSerializable})
	if err != nil {
		return modelToolClosureZeroV1(mapDBErrorV1(ctx, err, true))
	}
	defer func() { _ = tx.Rollback() }()
	result, err := tx.ExecContext(ctx, `
INSERT OR IGNORE INTO model_tool_injection_material_v1
(material_id,revision,digest,surface_id,surface_revision,surface_digest,compiled_tools_digest,expires_unix_nano,compiled_tools_json,body_json,row_digest)
VALUES(?,?,?,?,?,?,?,?,?,?,?)`,
		material.Ref.ID, int64(material.Ref.Revision), string(material.Ref.Digest),
		material.Surface.ID, int64(material.Surface.Revision), string(material.Surface.Digest),
		string(compiled.Digest), material.ExpiresUnixNano, compiledBody, materialBody, string(rowDigest),
	)
	if err != nil {
		return modelToolClosureZeroV1(mapDBErrorV1(ctx, err, true))
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return modelToolClosureZeroV1(mapDBErrorV1(ctx, err, true))
	}
	if affected == 0 {
		winnerCompiled, winnerMaterial, err := inspectMaterialTxV1(ctx, tx, material.Ref.ID)
		if err != nil {
			return modelToolClosureZeroV1(err)
		}
		if !reflect.DeepEqual(winnerCompiled, compiled) || !reflect.DeepEqual(winnerMaterial, material) {
			return modelToolClosureZeroV1(conflictV1("Model Tool Injection Material ID already binds a different compiled closure"))
		}
	}
	if err := tx.Commit(); err != nil {
		return modelToolClosureZeroV1(indeterminateV1("Model Tool Injection Material commit outcome is unknown"))
	}
	return compiled.Clone(), material.Clone(), nil
}

func (s *StoreV1) InspectExactModelToolInjectionMaterialV1(ctx context.Context, exact toolcontract.ModelToolInjectionMaterialRefV1) (toolcontract.ModelToolInjectionMaterialV1, error) {
	if err := s.readReadyV1(ctx); err != nil {
		return toolcontract.ModelToolInjectionMaterialV1{}, err
	}
	if err := exact.Validate(); err != nil {
		return toolcontract.ModelToolInjectionMaterialV1{}, err
	}
	_, material, err := inspectMaterialQueryV1(ctx, s.db, exact.ID)
	if err != nil {
		return toolcontract.ModelToolInjectionMaterialV1{}, err
	}
	if material.Ref != exact {
		return toolcontract.ModelToolInjectionMaterialV1{}, conflictV1("Model Tool Injection Material exact Ref drifted")
	}
	if err := material.ValidateCurrent(exact, s.clock()); err != nil {
		return toolcontract.ModelToolInjectionMaterialV1{}, err
	}
	return material.Clone(), nil
}

func (s *StoreV1) EnsureToolSurfaceInvocationBindingV1(ctx context.Context, request toolcontract.ToolSurfaceInvocationBindingEnsureRequestV1) (toolcontract.ToolSurfaceInvocationBindingV1, toolcontract.ToolSurfaceInvocationBindingAckV1, error) {
	if err := s.writeReadyV1(ctx); err != nil {
		return bindingZeroV1(err)
	}
	request, err := cloneBindingRequestV1(request)
	if err != nil {
		return bindingZeroV1(err)
	}
	if err := request.Validate(); err != nil {
		return bindingZeroV1(err)
	}
	requestDigest, err := request.ComputeDigest()
	if err != nil {
		return bindingZeroV1(err)
	}
	first := s.clock()
	if err := validateBindingRequestCurrentV1(request, first); err != nil {
		return bindingZeroV1(err)
	}
	release := s.acquireBindingInvocationGateV1(request.Invocation.InvocationID + "\x00" + string(request.Invocation.InvocationDigest))
	defer release()
	if err := contextErrorV1(ctx); err != nil {
		return bindingZeroV1(err)
	}
	second := s.clock()
	if second.Before(first) {
		return bindingZeroV1(core.NewError(core.ErrorPreconditionFailed, core.ReasonClockRegression, "Tool Surface Invocation Binding clock regressed"))
	}
	if err := validateBindingRequestCurrentV1(request, second); err != nil {
		return bindingZeroV1(err)
	}
	subject, err := toolcontract.SealToolSurfaceInvocationBindingSubjectV1(request)
	if err != nil {
		return bindingZeroV1(err)
	}
	deadline, _ := ctx.Deadline()
	notAfter := toolcontract.ToolSurfaceInvocationBindingNotAfterV1(subject, deadline)
	if second.UnixNano() >= notAfter {
		return bindingZeroV1(core.NewError(core.ErrorPreconditionFailed, core.ReasonBindingExpired, "Tool Surface Invocation Binding expired before commit"))
	}
	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelSerializable})
	if err != nil {
		return bindingZeroV1(mapDBErrorV1(ctx, err, true))
	}
	defer func() { _ = tx.Rollback() }()
	commitNow := s.clock()
	if commitNow.Before(second) {
		return bindingZeroV1(core.NewError(core.ErrorPreconditionFailed, core.ReasonClockRegression, "Tool Surface Invocation Binding commit clock regressed"))
	}
	if err := validateBindingRequestCurrentV1(request, commitNow); err != nil {
		return bindingZeroV1(err)
	}
	if commitNow.UnixNano() >= notAfter {
		return bindingZeroV1(core.NewError(core.ErrorPreconditionFailed, core.ReasonBindingExpired, "Tool Surface Invocation Binding expired at commit"))
	}

	if winnerRequest, winnerRequestDigest, winner, winnerAck, inspectErr := inspectBindingTxByInvocationV1(ctx, tx, request.Invocation); inspectErr == nil {
		if winnerRequestDigest != requestDigest || winnerRequest.ValidateAgainst(winner) != nil || request.ValidateAgainst(winner) != nil {
			return bindingZeroV1(core.NewError(core.ErrorConflict, core.ReasonIdempotencyPayloadMismatch, "Tool Surface Invocation Binding request differs from winner"))
		}
		if err := winner.ValidateCurrent(commitNow); err != nil {
			return bindingZeroV1(err)
		}
		if err := winnerAck.ValidateAgainst(winner, commitNow); err != nil {
			return bindingZeroV1(err)
		}
		return winner, winnerAck, nil
	} else if !core.HasCategory(inspectErr, core.ErrorNotFound) {
		return bindingZeroV1(inspectErr)
	}

	binding, err := toolcontract.SealToolSurfaceInvocationBindingV1(toolcontract.ToolSurfaceInvocationBindingV1{
		Ref:              toolcontract.ToolSurfaceInvocationBindingRefV1{Owner: s.owner},
		Subject:          subject,
		CreatedUnixNano:  commitNow.UnixNano(),
		NotAfterUnixNano: notAfter,
	})
	if err != nil {
		return bindingZeroV1(err)
	}
	ack, err := toolcontract.SealToolSurfaceInvocationBindingAckV1(toolcontract.ToolSurfaceInvocationBindingAckV1{
		BindingRef: binding.Ref, Invocation: binding.Subject.Invocation,
		PreparedFactRef: binding.Subject.PreparedFactRef, PreparedCurrentRef: binding.Subject.PreparedCurrentRef,
		CheckedUnixNano: commitNow.UnixNano(), NotAfterUnixNano: binding.NotAfterUnixNano,
	})
	if err != nil {
		return bindingZeroV1(err)
	}
	if err := ack.ValidateAgainst(binding, commitNow); err != nil {
		return bindingZeroV1(err)
	}
	requestBody, err := json.Marshal(request)
	if err != nil {
		return bindingZeroV1(invalidV1("Tool Surface Invocation Binding request JSON encode failed"))
	}
	bindingBody, err := json.Marshal(binding)
	if err != nil {
		return bindingZeroV1(invalidV1("Tool Surface Invocation Binding JSON encode failed"))
	}
	ackBody, err := json.Marshal(ack)
	if err != nil {
		return bindingZeroV1(invalidV1("Tool Surface Invocation Binding Ack JSON encode failed"))
	}
	rowDigest, err := bindingRowDigestV1(request, requestDigest, binding, ack)
	if err != nil {
		return bindingZeroV1(err)
	}
	result, err := tx.ExecContext(ctx, `
	INSERT OR IGNORE INTO tool_surface_invocation_binding_v1
	(binding_id,revision,digest,invocation_id,invocation_digest,expires_unix_nano,request_digest,request_json,binding_json,ack_json,row_digest)
	VALUES(?,?,?,?,?,?,?,?,?,?,?)`,
		binding.Ref.ID, int64(binding.Ref.Revision), string(binding.Ref.Digest),
		binding.Subject.Invocation.InvocationID, string(binding.Subject.Invocation.InvocationDigest),
		binding.NotAfterUnixNano, string(requestDigest), requestBody, bindingBody, ackBody, string(rowDigest),
	)
	if err != nil {
		return bindingZeroV1(mapDBErrorV1(ctx, err, true))
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return bindingZeroV1(mapDBErrorV1(ctx, err, true))
	}
	if affected == 0 {
		winnerRequest, winnerRequestDigest, winner, winnerAck, err := inspectBindingTxByInvocationV1(ctx, tx, request.Invocation)
		if err != nil {
			return bindingZeroV1(err)
		}
		if winnerRequestDigest != requestDigest || winnerRequest.ValidateAgainst(winner) != nil || request.ValidateAgainst(winner) != nil {
			return bindingZeroV1(core.NewError(core.ErrorConflict, core.ReasonIdempotencyPayloadMismatch, "Tool Surface Invocation Binding request differs from winner"))
		}
		if err := winner.ValidateCurrent(commitNow); err != nil {
			return bindingZeroV1(err)
		}
		if err := winnerAck.ValidateAgainst(winner, commitNow); err != nil {
			return bindingZeroV1(err)
		}
		return winner, winnerAck, nil
	}
	if err := tx.Commit(); err != nil {
		return bindingZeroV1(indeterminateV1("Tool Surface Invocation Binding commit outcome is unknown"))
	}
	return binding, ack, nil
}

func (s *StoreV1) InspectToolSurfaceInvocationBindingByInvocationV1(ctx context.Context, invocation toolcontract.ToolSurfaceInvocationCoordinateV1) (toolcontract.ToolSurfaceInvocationBindingV1, toolcontract.ToolSurfaceInvocationBindingAckV1, error) {
	if err := s.readReadyV1(ctx); err != nil {
		return bindingZeroV1(err)
	}
	if err := invocation.Validate(); err != nil {
		return bindingZeroV1(err)
	}
	row := s.db.QueryRowContext(ctx, `
	SELECT binding_id,revision,digest,invocation_id,invocation_digest,expires_unix_nano,request_digest,request_json,binding_json,ack_json,row_digest
	FROM tool_surface_invocation_binding_v1
	WHERE invocation_id=? AND invocation_digest=?`, invocation.InvocationID, string(invocation.InvocationDigest))
	_, _, binding, ack, err := decodeBindingRowV1(ctx, row)
	if err != nil {
		return bindingZeroV1(err)
	}
	if binding.Subject.Invocation != invocation {
		return bindingZeroV1(conflictV1("Tool Surface Invocation Binding invocation coordinate drifted"))
	}
	return s.validateBindingReadV1(binding, ack)
}

func (s *StoreV1) InspectExactToolSurfaceInvocationBindingV1(ctx context.Context, exact toolcontract.ToolSurfaceInvocationBindingRefV1) (toolcontract.ToolSurfaceInvocationBindingV1, toolcontract.ToolSurfaceInvocationBindingAckV1, error) {
	if err := s.readReadyV1(ctx); err != nil {
		return bindingZeroV1(err)
	}
	if err := exact.Validate(); err != nil {
		return bindingZeroV1(err)
	}
	row := s.db.QueryRowContext(ctx, `
	SELECT binding_id,revision,digest,invocation_id,invocation_digest,expires_unix_nano,request_digest,request_json,binding_json,ack_json,row_digest
	FROM tool_surface_invocation_binding_v1 WHERE binding_id=?`, exact.ID)
	_, _, binding, ack, err := decodeBindingRowV1(ctx, row)
	if err != nil {
		return bindingZeroV1(err)
	}
	if binding.Ref != exact {
		return bindingZeroV1(conflictV1("Tool Surface Invocation Binding exact Ref drifted"))
	}
	return s.validateBindingReadV1(binding, ack)
}

func (s *StoreV1) validateBindingReadV1(binding toolcontract.ToolSurfaceInvocationBindingV1, ack toolcontract.ToolSurfaceInvocationBindingAckV1) (toolcontract.ToolSurfaceInvocationBindingV1, toolcontract.ToolSurfaceInvocationBindingAckV1, error) {
	now := s.clock()
	if err := binding.ValidateCurrent(now); err != nil {
		return bindingZeroV1(err)
	}
	if err := ack.ValidateAgainst(binding, now); err != nil {
		return bindingZeroV1(err)
	}
	return binding, ack, nil
}

func (s *StoreV1) migrateV1(ctx context.Context) error {
	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelSerializable})
	if err != nil {
		return mapDBErrorV1(ctx, err, true)
	}
	defer func() { _ = tx.Rollback() }()
	if _, err := tx.ExecContext(ctx, schemaV1); err != nil {
		return mapDBErrorV1(ctx, err, true)
	}
	now := s.clock()
	if now.IsZero() || now.UnixNano() <= 0 {
		return core.NewError(core.ErrorPreconditionFailed, core.ReasonClockRegression, "Tool SQLite migration clock is invalid")
	}
	digest := core.DigestBytes([]byte(schemaV1))
	result, err := tx.ExecContext(ctx, `INSERT OR IGNORE INTO tool_owner_schema_v1(version,digest,applied_unix_nano) VALUES(?,?,?)`, schemaVersionV1, string(digest), now.UnixNano())
	if err != nil {
		return mapDBErrorV1(ctx, err, true)
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return mapDBErrorV1(ctx, err, true)
	}
	if affected == 0 {
		var stored string
		if err := tx.QueryRowContext(ctx, `SELECT digest FROM tool_owner_schema_v1 WHERE version=?`, schemaVersionV1).Scan(&stored); err != nil {
			return mapDBErrorV1(ctx, err, false)
		}
		if stored != string(digest) {
			return conflictV1("Tool SQLite schema digest drifted")
		}
	}
	if err := tx.Commit(); err != nil {
		return indeterminateV1("Tool SQLite migration commit outcome is unknown")
	}
	return nil
}

func (s *StoreV1) verifyV1(ctx context.Context) error {
	var journal string
	if err := s.db.QueryRowContext(ctx, `PRAGMA journal_mode`).Scan(&journal); err != nil {
		return mapDBErrorV1(ctx, err, false)
	}
	if !strings.EqualFold(journal, "wal") {
		return conflictV1("Tool SQLite WAL mode is inactive")
	}
	var foreignKeys int
	if err := s.db.QueryRowContext(ctx, `PRAGMA foreign_keys`).Scan(&foreignKeys); err != nil {
		return mapDBErrorV1(ctx, err, false)
	}
	if foreignKeys != 1 {
		return conflictV1("Tool SQLite foreign keys are inactive")
	}
	var synchronous int
	if err := s.db.QueryRowContext(ctx, `PRAGMA synchronous`).Scan(&synchronous); err != nil {
		return mapDBErrorV1(ctx, err, false)
	}
	if synchronous != 2 {
		return conflictV1("Tool SQLite FULL synchronous mode is inactive")
	}
	return nil
}

func (s *StoreV1) readReadyV1(ctx context.Context) error {
	if err := contextErrorV1(ctx); err != nil {
		return err
	}
	if s == nil || s.db == nil || s.clock == nil || s.owner.Validate() != nil {
		return unavailableV1("Tool SQLite exact Reader is unavailable")
	}
	return nil
}

func (s *StoreV1) writeReadyV1(ctx context.Context) error {
	return s.readReadyV1(ctx)
}

type queryRowerV1 interface {
	QueryRowContext(context.Context, string, ...any) *sql.Row
}

func inspectMaterialTxV1(ctx context.Context, tx *sql.Tx, id string) (toolsurface.CompiledModelToolsV1, toolcontract.ModelToolInjectionMaterialV1, error) {
	return inspectMaterialQueryV1(ctx, tx, id)
}

func inspectMaterialQueryV1(ctx context.Context, query queryRowerV1, id string) (toolsurface.CompiledModelToolsV1, toolcontract.ModelToolInjectionMaterialV1, error) {
	var compiledBody, materialBody []byte
	var storedID, storedDigest, surfaceID, surfaceDigest, compiledDigest, rowDigest string
	var storedRevision, surfaceRevision, expiresUnixNano int64
	if err := query.QueryRowContext(ctx, `
SELECT material_id,revision,digest,surface_id,surface_revision,surface_digest,compiled_tools_digest,expires_unix_nano,compiled_tools_json,body_json,row_digest
FROM model_tool_injection_material_v1 WHERE material_id=?`, id).Scan(
		&storedID, &storedRevision, &storedDigest, &surfaceID, &surfaceRevision, &surfaceDigest,
		&compiledDigest, &expiresUnixNano, &compiledBody, &materialBody, &rowDigest,
	); err != nil {
		return modelToolClosureZeroV1(mapDBErrorV1(ctx, err, false))
	}
	var compiled toolsurface.CompiledModelToolsV1
	var material toolcontract.ModelToolInjectionMaterialV1
	if core.DecodeStrictJSON(compiledBody, &compiled) != nil || core.DecodeStrictJSON(materialBody, &material) != nil ||
		!canonicalJSONBytesV1(compiledBody, compiled) || !canonicalJSONBytesV1(materialBody, material) {
		return modelToolClosureZeroV1(conflictV1("stored Model Tool Injection closure JSON is non-canonical"))
	}
	expected, err := modelToolClosureRowDigestV1(compiled, material)
	if err != nil || string(expected) != rowDigest || compiled.ValidateAgainstMaterialV1(material) != nil ||
		storedID != material.Ref.ID || storedRevision != int64(material.Ref.Revision) || storedDigest != string(material.Ref.Digest) ||
		surfaceID != material.Surface.ID || surfaceRevision != int64(material.Surface.Revision) ||
		surfaceDigest != string(material.Surface.Digest) || compiledDigest != string(compiled.Digest) ||
		expiresUnixNano != material.ExpiresUnixNano {
		return modelToolClosureZeroV1(conflictV1("stored Model Tool Injection closure row drifted"))
	}
	return compiled.Clone(), material.Clone(), nil
}

func inspectBindingTxByInvocationV1(ctx context.Context, tx *sql.Tx, invocation toolcontract.ToolSurfaceInvocationCoordinateV1) (toolcontract.ToolSurfaceInvocationBindingEnsureRequestV1, core.Digest, toolcontract.ToolSurfaceInvocationBindingV1, toolcontract.ToolSurfaceInvocationBindingAckV1, error) {
	row := tx.QueryRowContext(ctx, `
	SELECT binding_id,revision,digest,invocation_id,invocation_digest,expires_unix_nano,request_digest,request_json,binding_json,ack_json,row_digest
	FROM tool_surface_invocation_binding_v1 WHERE invocation_id=? AND invocation_digest=?`,
		invocation.InvocationID, string(invocation.InvocationDigest))
	return decodeBindingRowV1(ctx, row)
}

type scanRowV1 interface {
	Scan(...any) error
}

func decodeBindingRowV1(ctx context.Context, row scanRowV1) (toolcontract.ToolSurfaceInvocationBindingEnsureRequestV1, core.Digest, toolcontract.ToolSurfaceInvocationBindingV1, toolcontract.ToolSurfaceInvocationBindingAckV1, error) {
	var requestBody, bindingBody, ackBody []byte
	var bindingID, bindingDigest, invocationID, invocationDigest, requestDigestText, storedRowDigest string
	var revision, expiresUnixNano int64
	if err := row.Scan(
		&bindingID, &revision, &bindingDigest, &invocationID, &invocationDigest, &expiresUnixNano,
		&requestDigestText, &requestBody, &bindingBody, &ackBody, &storedRowDigest,
	); err != nil {
		return bindingRecordZeroV1(mapDBErrorV1(ctx, err, false))
	}
	var request toolcontract.ToolSurfaceInvocationBindingEnsureRequestV1
	var binding toolcontract.ToolSurfaceInvocationBindingV1
	var ack toolcontract.ToolSurfaceInvocationBindingAckV1
	if core.DecodeStrictJSON(requestBody, &request) != nil || core.DecodeStrictJSON(bindingBody, &binding) != nil || core.DecodeStrictJSON(ackBody, &ack) != nil ||
		!canonicalJSONBytesV1(requestBody, request) || !canonicalJSONBytesV1(bindingBody, binding) || !canonicalJSONBytesV1(ackBody, ack) ||
		request.Validate() != nil || binding.Validate() != nil || ack.Validate() != nil {
		return bindingRecordZeroV1(conflictV1("stored Tool Surface Invocation Binding row is non-canonical"))
	}
	requestDigest, err := request.ComputeDigest()
	if err != nil || string(requestDigest) != requestDigestText || request.ValidateAgainst(binding) != nil {
		return bindingRecordZeroV1(conflictV1("stored Tool Surface Invocation Binding request drifted"))
	}
	rowDigest, err := bindingRowDigestV1(request, requestDigest, binding, ack)
	if err != nil || string(rowDigest) != storedRowDigest ||
		bindingID != binding.Ref.ID || revision != int64(binding.Ref.Revision) ||
		bindingDigest != string(binding.Ref.Digest) ||
		invocationID != binding.Subject.Invocation.InvocationID ||
		invocationDigest != string(binding.Subject.Invocation.InvocationDigest) ||
		expiresUnixNano != binding.NotAfterUnixNano {
		return bindingRecordZeroV1(conflictV1("stored Tool Surface Invocation Binding row digest drifted"))
	}
	return request, requestDigest, binding, ack, nil
}

func encodeRowV1(discriminator string, value any) ([]byte, core.Digest, error) {
	body, err := json.Marshal(value)
	if err != nil {
		return nil, "", invalidV1("Tool SQLite row JSON encode failed")
	}
	digest, err := rowDigestV1(discriminator, value)
	return body, digest, err
}

func rowDigestV1(discriminator string, value any) (core.Digest, error) {
	return core.CanonicalJSONDigest("praxis.tool-mcp.sqlite-row", "v1", discriminator, value)
}

func modelToolClosureRowDigestV1(compiled toolsurface.CompiledModelToolsV1, material toolcontract.ModelToolInjectionMaterialV1) (core.Digest, error) {
	return rowDigestV1("ModelToolInjectionClosureV1", struct {
		Compiled toolsurface.CompiledModelToolsV1          `json:"compiled"`
		Material toolcontract.ModelToolInjectionMaterialV1 `json:"material"`
	}{Compiled: compiled, Material: material})
}

func bindingRowDigestV1(request toolcontract.ToolSurfaceInvocationBindingEnsureRequestV1, requestDigest core.Digest, binding toolcontract.ToolSurfaceInvocationBindingV1, ack toolcontract.ToolSurfaceInvocationBindingAckV1) (core.Digest, error) {
	return rowDigestV1("ToolSurfaceInvocationBindingRowV1", struct {
		Request       toolcontract.ToolSurfaceInvocationBindingEnsureRequestV1 `json:"request"`
		RequestDigest core.Digest                                              `json:"request_digest"`
		Binding       toolcontract.ToolSurfaceInvocationBindingV1              `json:"binding"`
		Ack           toolcontract.ToolSurfaceInvocationBindingAckV1           `json:"ack"`
	}{Request: request, RequestDigest: requestDigest, Binding: binding, Ack: ack})
}

func canonicalJSONBytesV1(stored []byte, value any) bool {
	canonical, err := json.Marshal(value)
	return err == nil && bytes.Equal(stored, canonical)
}

func cloneBindingRequestV1(request toolcontract.ToolSurfaceInvocationBindingEnsureRequestV1) (toolcontract.ToolSurfaceInvocationBindingEnsureRequestV1, error) {
	body, err := json.Marshal(request)
	if err != nil {
		return toolcontract.ToolSurfaceInvocationBindingEnsureRequestV1{}, invalidV1("Tool Surface Invocation Binding request JSON encode failed")
	}
	var cloned toolcontract.ToolSurfaceInvocationBindingEnsureRequestV1
	if err := core.DecodeStrictJSON(body, &cloned); err != nil {
		return toolcontract.ToolSurfaceInvocationBindingEnsureRequestV1{}, invalidV1("Tool Surface Invocation Binding request JSON clone failed")
	}
	return cloned, nil
}

func (s *StoreV1) acquireBindingInvocationGateV1(key string) func() {
	s.gatesMu.Lock()
	gate := s.gates[key]
	if gate == nil {
		gate = &bindingInvocationGateV1{}
		s.gates[key] = gate
	}
	gate.refs++
	s.gatesMu.Unlock()
	gate.mu.Lock()
	return func() {
		gate.mu.Unlock()
		s.gatesMu.Lock()
		gate.refs--
		if gate.refs == 0 && s.gates[key] == gate {
			delete(s.gates, key)
		}
		s.gatesMu.Unlock()
	}
}

func contextErrorV1(ctx context.Context) error {
	if ctx == nil {
		return invalidV1("Tool SQLite context is required")
	}
	return ctx.Err()
}

func validateBindingRequestCurrentV1(request toolcontract.ToolSurfaceInvocationBindingEnsureRequestV1, now time.Time) error {
	if now.IsZero() {
		return core.NewError(core.ErrorPreconditionFailed, core.ReasonClockRegression, "Tool Surface Invocation Binding clock is unavailable")
	}
	if err := request.PreparedCurrent.ValidateCurrent(request.PreparedCurrentRef, now); err != nil {
		return err
	}
	if err := request.SurfaceCurrent.ValidateCurrent(request.SurfaceCurrent.Ref, now); err != nil {
		return err
	}
	if err := request.AssemblyCurrent.ValidateCurrent(request.AssemblyCurrentRef, now); err != nil {
		return err
	}
	if !now.Before(time.Unix(0, request.RequestedNotAfterUnixNano)) || !now.Before(time.Unix(0, request.PreparedHistoricalFact.NotAfterUnixNano)) {
		return core.NewError(core.ErrorPreconditionFailed, core.ReasonBindingExpired, "Tool Surface Invocation Binding request is no longer current")
	}
	return nil
}

func mapDBErrorV1(ctx context.Context, err error, mutation bool) error {
	if errors.Is(err, sql.ErrNoRows) {
		return core.NewError(core.ErrorNotFound, core.ReasonInvalidReference, "Tool SQLite exact row is absent")
	}
	if ctx == nil || ctx.Err() != nil || errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return indeterminateV1("Tool SQLite operation outcome is indeterminate")
	}
	message := strings.ToLower(err.Error())
	if strings.Contains(message, "busy") || strings.Contains(message, "locked") {
		return unavailableV1("Tool SQLite is busy")
	}
	if strings.Contains(message, "constraint") || strings.Contains(message, "unique") {
		return conflictV1("Tool SQLite uniqueness conflict")
	}
	if mutation {
		return indeterminateV1("Tool SQLite mutation outcome is unknown")
	}
	return unavailableV1("Tool SQLite read failed")
}

func bindingZeroV1(err error) (toolcontract.ToolSurfaceInvocationBindingV1, toolcontract.ToolSurfaceInvocationBindingAckV1, error) {
	return toolcontract.ToolSurfaceInvocationBindingV1{}, toolcontract.ToolSurfaceInvocationBindingAckV1{}, err
}

func bindingRecordZeroV1(err error) (toolcontract.ToolSurfaceInvocationBindingEnsureRequestV1, core.Digest, toolcontract.ToolSurfaceInvocationBindingV1, toolcontract.ToolSurfaceInvocationBindingAckV1, error) {
	return toolcontract.ToolSurfaceInvocationBindingEnsureRequestV1{}, "", toolcontract.ToolSurfaceInvocationBindingV1{}, toolcontract.ToolSurfaceInvocationBindingAckV1{}, err
}

func modelToolClosureZeroV1(err error) (toolsurface.CompiledModelToolsV1, toolcontract.ModelToolInjectionMaterialV1, error) {
	return toolsurface.CompiledModelToolsV1{}, toolcontract.ModelToolInjectionMaterialV1{}, err
}

func invalidV1(message string) error {
	return core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidCanonicalForm, message)
}

func conflictV1(message string) error {
	return core.NewError(core.ErrorConflict, core.ReasonBindingDrift, message)
}

func unavailableV1(message string) error {
	return core.NewError(core.ErrorUnavailable, core.ReasonComponentMissing, message)
}

func indeterminateV1(message string) error {
	return core.NewError(core.ErrorIndeterminate, core.ReasonEffectUnknownOutcome, message)
}

var _ toolcontract.ModelToolInjectionMaterialReaderV1 = (*StoreV1)(nil)
var _ toolcontract.ToolSurfaceInvocationBindingRepositoryV1 = (*StoreV1)(nil)

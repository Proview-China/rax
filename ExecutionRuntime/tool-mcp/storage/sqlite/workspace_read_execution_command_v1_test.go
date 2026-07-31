package sqlite

import (
	"context"
	"errors"
	"path/filepath"
	"reflect"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	applicationcontract "github.com/Proview-China/rax/ExecutionRuntime/application/contract"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	toolcontract "github.com/Proview-China/rax/ExecutionRuntime/tool-mcp/contract"
	ownercommand "github.com/Proview-China/rax/ExecutionRuntime/tool-mcp/internal/owner/workspacereadcommandrepo"
	"github.com/Proview-China/rax/ExecutionRuntime/tool-mcp/internal/testkit"
)

func TestWorkspaceReadExecutionCommandSQLiteCreateExactReverseRestartAndStableCreatedV1(t *testing.T) {
	ctx := context.Background()
	now := testkit.FixedTime.Add(3 * time.Second)
	clock := testkit.NewManualClock(now)
	path := filepath.Join(t.TempDir(), "workspace-read-command.db")
	store := openWorkspaceReadExecutionCommandStoreV1(t, path, clock.Now)
	fact := workspaceReadExecutionCommandFactV1(t, now, "restart")

	winner, created, err := store.CreateWorkspaceReadExecutionCommandOwnedV1(
		ctx, ownercommand.NewWriteCapabilityV1(), fact,
	)
	if err != nil || !created || !reflect.DeepEqual(winner, fact) {
		t.Fatalf("create winner=%#v created=%v err=%v", winner.Ref, created, err)
	}
	exact, err := store.InspectWorkspaceReadExecutionCommandExactV1(ctx, fact.Ref)
	if err != nil || !reflect.DeepEqual(exact, fact) {
		t.Fatalf("exact Inspect changed command: equal=%v err=%v", reflect.DeepEqual(exact, fact), err)
	}
	reverse, err := store.InspectWorkspaceReadExecutionCommandByRuntimeAttemptV1(ctx, fact.RuntimeAttempt)
	if err != nil || !reflect.DeepEqual(reverse, fact) {
		t.Fatalf("full Runtime Attempt reverse changed command: equal=%v err=%v", reflect.DeepEqual(reverse, fact), err)
	}

	later := resealWorkspaceReadExecutionCommandFactV1(t, fact, func(value *toolcontract.WorkspaceReadExecutionCommandV1) {
		value.CreatedUnixNano++
	})
	converged, created, err := store.CreateWorkspaceReadExecutionCommandOwnedV1(
		ctx, ownercommand.NewWriteCapabilityV1(), later,
	)
	if err != nil || created || !reflect.DeepEqual(converged, fact) {
		t.Fatalf("stable closure with another Created did not converge: equal=%v created=%v err=%v", reflect.DeepEqual(converged, fact), created, err)
	}
	if err = store.Close(); err != nil {
		t.Fatal(err)
	}

	store = openWorkspaceReadExecutionCommandStoreV1(t, path, clock.Now)
	defer store.Close()
	restarted, err := store.InspectWorkspaceReadExecutionCommandExactV1(ctx, fact.Ref)
	if err != nil || !reflect.DeepEqual(restarted, fact) {
		t.Fatalf("restart exact Inspect changed command: equal=%v err=%v", reflect.DeepEqual(restarted, fact), err)
	}
	restartedReverse, err := store.InspectWorkspaceReadExecutionCommandByRuntimeAttemptV1(ctx, fact.RuntimeAttempt)
	if err != nil || !reflect.DeepEqual(restartedReverse, fact) {
		t.Fatalf("restart Runtime Attempt reverse changed command: equal=%v err=%v", reflect.DeepEqual(restartedReverse, fact), err)
	}
}

func TestWorkspaceReadExecutionCommandSQLite64MultiHandleIncreasingCreatedSingleWinnerV1(t *testing.T) {
	ctx := context.Background()
	now := testkit.FixedTime.Add(4 * time.Second)
	path := filepath.Join(t.TempDir(), "workspace-read-command-concurrent.db")
	var ticks atomic.Int64
	clock := func() time.Time {
		return now.Add(time.Second + time.Duration(ticks.Add(1))*time.Nanosecond)
	}

	const workers = 64
	stores := make([]*WorkspaceReadExecutionCommandStoreV1, workers)
	for i := range stores {
		stores[i] = openWorkspaceReadExecutionCommandStoreV1(t, path, clock)
		defer stores[i].Close()
	}
	base := workspaceReadExecutionCommandFactV1(t, now, "concurrent")
	facts := make([]toolcontract.WorkspaceReadExecutionCommandV1, workers)
	for i := range facts {
		facts[i] = resealWorkspaceReadExecutionCommandFactV1(t, base, func(value *toolcontract.WorkspaceReadExecutionCommandV1) {
			value.CreatedUnixNano += int64(i + 1)
		})
	}

	start := make(chan struct{})
	results := make(chan toolcontract.WorkspaceReadExecutionCommandV1, workers)
	errs := make(chan error, workers)
	var createdCount atomic.Int64
	var wg sync.WaitGroup
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func(index int) {
			defer wg.Done()
			<-start
			winner, created, err := stores[index].CreateWorkspaceReadExecutionCommandOwnedV1(
				ctx, ownercommand.NewWriteCapabilityV1(), facts[index],
			)
			if created {
				createdCount.Add(1)
			}
			if err == nil {
				results <- winner
			}
			errs <- err
		}(i)
	}
	close(start)
	wg.Wait()
	close(results)
	close(errs)

	for err := range errs {
		if err != nil {
			t.Fatal(err)
		}
	}
	if got := createdCount.Load(); got != 1 {
		t.Fatalf("created winners=%d, want 1", got)
	}
	var winner toolcontract.WorkspaceReadExecutionCommandV1
	count := 0
	for result := range results {
		count++
		if winner.Ref == (toolcontract.WorkspaceReadExecutionCommandRefV1{}) {
			winner = result
			continue
		}
		if !reflect.DeepEqual(winner, result) {
			t.Fatal("64 multi-handle contenders did not converge to one exact winner")
		}
	}
	if count != workers {
		t.Fatalf("result count=%d, want %d", count, workers)
	}
	var rows int
	if err := stores[0].store.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM tool_workspace_read_execution_command_v1`).Scan(&rows); err != nil {
		t.Fatal(err)
	}
	if rows != 1 {
		t.Fatalf("durable rows=%d, want 1", rows)
	}
}

func TestWorkspaceReadExecutionCommandSQLiteUniqueAxesFailClosedV1(t *testing.T) {
	now := testkit.FixedTime.Add(5 * time.Second)
	base := workspaceReadExecutionCommandFactV1(t, now, "axis-a")
	other := workspaceReadExecutionCommandFactV1(t, now, "axis-b")

	t.Run("request key only", func(t *testing.T) {
		requestOnly := other
		source := other.Source
		source.RequestKey = base.Source.RequestKey
		source.ClaimRef = base.Source.ClaimRef
		source.ExecutionInputDigest = testkit.Digest("request-only-input")
		source.ExecutionStateRef.ID = mustStableIDV1(t, "tool-owner-execution-state-v2", source.ClaimRef.ID, string(source.ExecutionInputDigest))
		source.ExecutionStateRef.Digest = testkit.Digest("request-only-state")
		source.ToolExecutionAttemptID = mustStableIDV1(t, "tool-owner-execution-attempt-v2", source.ClaimRef.ID, string(source.ExecutionInputDigest))
		requestOnly = resealWorkspaceReadExecutionCommandSourceAndFactV1(t, requestOnly, source)
		assertWorkspaceReadExecutionCommandConflictV1(t, base, requestOnly)
	})

	t.Run("Tool execution attempt only", func(t *testing.T) {
		toolAttemptOnly := other
		source := base.Source
		key := source.RequestKey
		key.ActionCoordinateDigest = testkit.Digest("axis-tool-only-action")
		key = resealWorkspaceReadExecutionCommandRequestKeyV1(t, key)
		source.RequestKey = key
		toolAttemptOnly = resealWorkspaceReadExecutionCommandSourceAndFactV1(t, toolAttemptOnly, source)
		assertWorkspaceReadExecutionCommandConflictV1(t, base, toolAttemptOnly)
	})

	t.Run("same Runtime Attempt different request", func(t *testing.T) {
		runtimeOnly := other
		runtimeOnly.Operation = base.Operation
		runtimeOnly.OperationDigest = base.OperationDigest
		runtimeOnly.Prepared = base.Prepared
		runtimeOnly.PreparedSemanticDigest = base.PreparedSemanticDigest
		runtimeOnly.RuntimeAttempt = base.RuntimeAttempt
		runtimeOnly.RuntimeAttemptDigest = base.RuntimeAttemptDigest
		runtimeOnly.RuntimeEffectIntentDigest = base.RuntimeEffectIntentDigest
		runtimeOnly = resealWorkspaceReadExecutionCommandFactV1(t, runtimeOnly, nil)
		assertWorkspaceReadExecutionCommandConflictV1(t, base, runtimeOnly)
	})

	t.Run("same exact axes different stable closure", func(t *testing.T) {
		exactConflict := resealWorkspaceReadExecutionCommandFactV1(t, base, func(value *toolcontract.WorkspaceReadExecutionCommandV1) {
			value.RuntimeEffectFactRevision++
		})
		assertWorkspaceReadExecutionCommandConflictV1(t, base, exactConflict)
	})

	t.Run("axes split across two winners", func(t *testing.T) {
		ctx := context.Background()
		clock := testkit.NewManualClock(now)
		store := openWorkspaceReadExecutionCommandStoreV1(t, filepath.Join(t.TempDir(), "split.db"), clock.Now)
		defer store.Close()
		for _, fact := range []toolcontract.WorkspaceReadExecutionCommandV1{base, other} {
			if _, created, err := store.CreateWorkspaceReadExecutionCommandOwnedV1(ctx, ownercommand.NewWriteCapabilityV1(), fact); err != nil || !created {
				t.Fatalf("seed create created=%v err=%v", created, err)
			}
		}
		split := other
		split.Source = base.Source
		split = resealWorkspaceReadExecutionCommandFactV1(t, split, nil)
		if _, _, err := store.CreateWorkspaceReadExecutionCommandOwnedV1(ctx, ownercommand.NewWriteCapabilityV1(), split); err == nil || !core.HasCategory(err, core.ErrorConflict) {
			t.Fatalf("split unique axes error=%v, want Conflict", err)
		}
	})
}

func TestWorkspaceReadExecutionCommandSQLiteExpiredHistoricalReplayAndLostReplyV1(t *testing.T) {
	for _, point := range []string{
		"workspace_read_execution_command_before_insert",
		"workspace_read_execution_command_before_commit",
	} {
		t.Run(point, func(t *testing.T) {
			ctx := context.Background()
			now := testkit.FixedTime.Add(5 * time.Second)
			store := openWorkspaceReadExecutionCommandStoreV1(
				t, filepath.Join(t.TempDir(), point+".db"), func() time.Time { return now },
			)
			defer store.Close()
			fact := workspaceReadExecutionCommandFactV1(t, now, point)
			store.fault = func(actual string) error {
				if actual == point {
					return errors.New("injected pre-commit failure")
				}
				return nil
			}
			if _, _, err := store.CreateWorkspaceReadExecutionCommandOwnedV1(
				ctx, ownercommand.NewWriteCapabilityV1(), fact,
			); err == nil || core.HasCategory(err, core.ErrorIndeterminate) {
				t.Fatalf("pre-commit error=%v, want known non-Indeterminate failure", err)
			}
			var rows int
			if err := store.store.db.QueryRowContext(
				ctx, `SELECT COUNT(*) FROM tool_workspace_read_execution_command_v1`,
			).Scan(&rows); err != nil {
				t.Fatal(err)
			}
			if rows != 0 {
				t.Fatalf("pre-commit failure persisted %d rows", rows)
			}
			if _, err := store.InspectWorkspaceReadExecutionCommandExactV1(ctx, fact.Ref); err == nil ||
				!core.HasCategory(err, core.ErrorNotFound) {
				t.Fatalf("pre-commit failure exact Inspect error=%v, want NotFound", err)
			}
		})
	}

	t.Run("before commit hook crosses exact expiry", func(t *testing.T) {
		ctx := context.Background()
		now := testkit.FixedTime.Add(5 * time.Second)
		clock := testkit.NewManualClock(now)
		store := openWorkspaceReadExecutionCommandStoreV1(
			t, filepath.Join(t.TempDir(), "ttl-crossing.db"), clock.Now,
		)
		defer store.Close()
		fact := workspaceReadExecutionCommandFactV1(t, now, "ttl-crossing")
		store.fault = func(point string) error {
			if point == "workspace_read_execution_command_before_commit" {
				clock.Set(time.Unix(0, fact.NotAfterUnixNano))
			}
			return nil
		}
		if _, _, err := store.CreateWorkspaceReadExecutionCommandOwnedV1(
			ctx, ownercommand.NewWriteCapabilityV1(), fact,
		); err == nil || !core.HasReason(err, core.ReasonBindingExpired) {
			t.Fatalf("TTL crossing error=%v, want BindingExpired", err)
		}
		var rows int
		if err := store.store.db.QueryRowContext(
			ctx, `SELECT COUNT(*) FROM tool_workspace_read_execution_command_v1`,
		).Scan(&rows); err != nil {
			t.Fatal(err)
		}
		if rows != 0 {
			t.Fatalf("TTL crossing persisted %d rows", rows)
		}
	})

	t.Run("expired historical replay", func(t *testing.T) {
		ctx := context.Background()
		now := testkit.FixedTime.Add(6 * time.Second)
		clock := testkit.NewManualClock(now)
		store := openWorkspaceReadExecutionCommandStoreV1(t, filepath.Join(t.TempDir(), "expired.db"), clock.Now)
		defer store.Close()
		fact := workspaceReadExecutionCommandFactV1(t, now, "expired")
		if _, created, err := store.CreateWorkspaceReadExecutionCommandOwnedV1(ctx, ownercommand.NewWriteCapabilityV1(), fact); err != nil || !created {
			t.Fatalf("seed create created=%v err=%v", created, err)
		}
		clock.Set(time.Unix(0, fact.NotAfterUnixNano))
		winner, created, err := store.CreateWorkspaceReadExecutionCommandOwnedV1(ctx, ownercommand.NewWriteCapabilityV1(), fact)
		if err != nil || created || !reflect.DeepEqual(winner, fact) {
			t.Fatalf("expired replay did not return historical winner: equal=%v created=%v err=%v", reflect.DeepEqual(winner, fact), created, err)
		}
		if _, err = store.InspectWorkspaceReadExecutionCommandExactV1(ctx, fact.Ref); err != nil {
			t.Fatalf("expired exact history became unreadable: %v", err)
		}
	})

	t.Run("commit lost reply", func(t *testing.T) {
		ctx := context.Background()
		now := testkit.FixedTime.Add(7 * time.Second)
		clock := testkit.NewManualClock(now)
		store := openWorkspaceReadExecutionCommandStoreV1(t, filepath.Join(t.TempDir(), "lost-reply.db"), clock.Now)
		defer store.Close()
		fact := workspaceReadExecutionCommandFactV1(t, now, "lost-reply")
		store.fault = func(point string) error {
			if point == "workspace_read_execution_command_after_commit" {
				return errors.New("lost reply")
			}
			return nil
		}
		if _, _, err := store.CreateWorkspaceReadExecutionCommandOwnedV1(ctx, ownercommand.NewWriteCapabilityV1(), fact); err == nil || !core.HasCategory(err, core.ErrorIndeterminate) {
			t.Fatalf("lost reply error=%v, want Indeterminate", err)
		}
		store.fault = nil
		recovered, err := store.InspectWorkspaceReadExecutionCommandExactV1(ctx, fact.Ref)
		if err != nil || !reflect.DeepEqual(recovered, fact) {
			t.Fatalf("exact Inspect did not recover committed winner: equal=%v err=%v", reflect.DeepEqual(recovered, fact), err)
		}
	})
}

func TestWorkspaceReadExecutionCommandSQLiteStoredSpliceAndSchemaDriftV1(t *testing.T) {
	for _, test := range []struct {
		name   string
		label  string
		update string
		arg    any
	}{
		{name: "flattened row", label: "flattened-row", update: `UPDATE tool_workspace_read_execution_command_v1 SET request_id=?`, arg: "spliced-request"},
		{name: "body", label: "body", update: `UPDATE tool_workspace_read_execution_command_v1 SET body_json=?`, arg: []byte(`{"spliced":true}`)},
		{name: "row digest", label: "row-digest", update: `UPDATE tool_workspace_read_execution_command_v1 SET row_digest=?`, arg: string(testkit.Digest("spliced-row"))},
	} {
		t.Run(test.name, func(t *testing.T) {
			ctx := context.Background()
			now := testkit.FixedTime.Add(8 * time.Second)
			store := openWorkspaceReadExecutionCommandStoreV1(t, filepath.Join(t.TempDir(), "splice.db"), func() time.Time { return now })
			defer store.Close()
			fact := workspaceReadExecutionCommandFactV1(t, now, "splice-"+test.label)
			if _, created, err := store.CreateWorkspaceReadExecutionCommandOwnedV1(ctx, ownercommand.NewWriteCapabilityV1(), fact); err != nil || !created {
				t.Fatalf("seed create created=%v err=%v", created, err)
			}
			if _, err := store.store.db.ExecContext(ctx, test.update+` WHERE command_id=?`, test.arg, fact.Ref.ID); err != nil {
				t.Fatal(err)
			}
			if _, err := store.InspectWorkspaceReadExecutionCommandExactV1(ctx, fact.Ref); err == nil || !core.HasCategory(err, core.ErrorConflict) {
				t.Fatalf("stored %s splice error=%v, want Conflict", test.name, err)
			}
		})
	}

	t.Run("physical schema", func(t *testing.T) {
		ctx := context.Background()
		now := testkit.FixedTime.Add(9 * time.Second)
		path := filepath.Join(t.TempDir(), "schema.db")
		store := openWorkspaceReadExecutionCommandStoreV1(t, path, func() time.Time { return now })
		if _, err := store.store.db.ExecContext(ctx, `ALTER TABLE tool_workspace_read_execution_command_v1 ADD COLUMN injected TEXT`); err != nil {
			t.Fatal(err)
		}
		if err := store.Close(); err != nil {
			t.Fatal(err)
		}
		reopened, err := OpenWorkspaceReadExecutionCommandStoreV1(ctx, ConfigV1{
			Path: path, Clock: func() time.Time { return now }, Owner: testkit.Owner(),
		})
		if reopened != nil {
			_ = reopened.Close()
		}
		if err == nil || !core.HasCategory(err, core.ErrorConflict) {
			t.Fatalf("physical schema drift reopen error=%v, want Conflict", err)
		}
	})
}

func workspaceReadExecutionCommandFactV1(
	t *testing.T,
	now time.Time,
	label string,
) toolcontract.WorkspaceReadExecutionCommandV1 {
	t.Helper()
	boundary := testkit.BoundaryFixture(now)
	attempt := boundary.Attempt
	delegation := *attempt.Delegation
	delegation.ID = "delegation-" + label
	delegation.Digest = testkit.Digest("delegation-current-" + label)
	attempt.Delegation = &delegation
	attempt.EffectID = core.EffectIntentID("effect-" + label)
	attempt.IntentDigest = testkit.Digest("intent-" + label)
	attempt.PermitID = "permit-" + label
	attempt.PermitDigest = testkit.Digest("permit-" + label)
	attempt.AttemptID = "attempt-" + label
	boundary.Attempt = attempt

	schema := testkit.Schema("workspace-read-command")
	payloadDigest := testkit.Digest("workspace-read-payload")
	prepared := testkit.PreparedAttemptFor(now, boundary, testkit.ProviderBinding(), schema, payloadDigest, 1)
	attemptDigest, err := toolcontract.DigestWorkspaceReadExecutionRuntimeAttemptV1(attempt)
	if err != nil {
		t.Fatal(err)
	}
	key := applicationcontract.SingleCallToolActionInspectKeyV2{
		ContractVersion:        applicationcontract.SingleCallToolActionContractVersionV2,
		RequestID:              "request-" + label,
		RequestRevision:        1,
		RequestDigest:          testkit.Digest("request-" + label),
		ActionCoordinateDigest: testkit.Digest("action-" + label),
		ScopeDigest:            boundary.Operation.ExecutionScopeDigest,
	}
	key = resealWorkspaceReadExecutionCommandRequestKeyV1(t, key)
	claimID := mustStableIDV1(
		t,
		"tool-owner-single-call-claim-v2",
		key.RequestID,
		string(key.RequestDigest),
		string(key.ScopeDigest),
	)
	inputDigest := testkit.Digest("execution-input-" + label)
	source, err := toolcontract.SealWorkspaceReadExecutionCommandSourceV1(
		toolcontract.WorkspaceReadExecutionCommandSourceV1{
			RequestKey:             key,
			ClaimRef:               toolcontract.ObjectRef{ID: claimID, Revision: 1, Digest: testkit.Digest("claim-" + label)},
			ExecutionStateRef:      toolcontract.ObjectRef{ID: mustStableIDV1(t, "tool-owner-execution-state-v2", claimID, string(inputDigest)), Revision: 1, Digest: testkit.Digest("state-" + label)},
			ExecutionStateKind:     toolcontract.WorkspaceReadExecutionStartCommittedV1,
			ExecutionInputDigest:   inputDigest,
			ToolExecutionAttemptID: mustStableIDV1(t, "tool-owner-execution-attempt-v2", claimID, string(inputDigest)),
			BindingCurrent:         toolcontract.SingleCallToolActionBindingCurrentRefV2{ID: "binding-" + label, Revision: 1, Digest: testkit.Digest("binding-" + label)},
			Candidate:              toolcontract.ObjectRef{ID: "candidate-" + label, Revision: 1, Digest: testkit.Digest("candidate-" + label)},
			CandidateClosureDigest: testkit.Digest("candidate-closure-" + label),
			InputContractCurrent:   toolcontract.ToolInputContractCurrentRefV1{ID: "input-current-" + label, Revision: 1, Digest: testkit.Digest("input-current-" + label)},
			Tool:                   toolcontract.ObjectRef{ID: "tool-" + label, Revision: 1, Digest: testkit.Digest("tool-" + label)},
			ToolCurrent: toolcontract.ToolRegistryObjectCurrentRefV1{
				Kind: toolcontract.ToolRegistryDescriptorCurrentKindV1, ID: "tool-current-" + label,
				Revision: 1, Digest: testkit.Digest("tool-current-" + label),
			},
			Owner: testkit.SettlementOwner(),
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	notAfter := now.Add(10 * time.Second).UnixNano()
	ttl, err := toolcontract.SealWorkspaceReadExecutionCommandTTLClosureV1(
		toolcontract.WorkspaceReadExecutionCommandTTLClosureV1{
			ClaimCreatedUnixNano:        now.Add(-2 * time.Second).UnixNano(),
			StateUpdatedUnixNano:        now.Add(-time.Second).UnixNano(),
			BindingCheckedUnixNano:      now.Add(-time.Second).UnixNano(),
			CandidateCreatedUnixNano:    now.Add(-2 * time.Second).UnixNano(),
			InputCheckedUnixNano:        now.Add(-time.Second).UnixNano(),
			PreparedUnixNano:            prepared.PreparedUnixNano,
			RequestedNotAfterUnixNano:   notAfter,
			RequestExpiresUnixNano:      notAfter,
			StateExpiresUnixNano:        notAfter,
			BindingExpiresUnixNano:      notAfter,
			CandidateExpiresUnixNano:    notAfter,
			InputExpiresUnixNano:        notAfter,
			EffectIntentExpiresUnixNano: notAfter,
			PreparedExpiresUnixNano:     prepared.ExpiresUnixNano,
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	fact, err := toolcontract.SealWorkspaceReadExecutionCommandV1(
		toolcontract.WorkspaceReadExecutionCommandV1{
			Source:                    source,
			Operation:                 boundary.Operation,
			OperationDigest:           attempt.OperationDigest,
			Prepared:                  prepared,
			PreparedSemanticDigest:    testkit.Digest("prepared-semantic-" + label),
			RuntimeAttempt:            attempt,
			RuntimeAttemptDigest:      attemptDigest,
			RuntimeEffectIntentDigest: attempt.IntentDigest,
			RuntimeEffectFactRevision: 1,
			RuntimeEffectState:        toolcontract.WorkspaceReadExecutionDispatchIntentV1,
			PayloadSchema:             schema,
			PayloadDigest:             payloadDigest,
			PayloadRevision:           1,
			TTL:                       ttl,
			CreatedUnixNano:           now.UnixNano(),
			NotAfterUnixNano:          ttl.EffectiveNotAfterUnixNano,
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	return fact
}

func openWorkspaceReadExecutionCommandStoreV1(
	t *testing.T,
	path string,
	clock func() time.Time,
) *WorkspaceReadExecutionCommandStoreV1 {
	t.Helper()
	store, err := OpenWorkspaceReadExecutionCommandStoreV1(context.Background(), ConfigV1{
		Path: path, Clock: clock, Owner: testkit.Owner(),
	})
	if err != nil {
		t.Fatal(err)
	}
	return store
}

func resealWorkspaceReadExecutionCommandFactV1(
	t *testing.T,
	fact toolcontract.WorkspaceReadExecutionCommandV1,
	mutate func(*toolcontract.WorkspaceReadExecutionCommandV1),
) toolcontract.WorkspaceReadExecutionCommandV1 {
	t.Helper()
	if mutate != nil {
		mutate(&fact)
	}
	fact.Ref = toolcontract.WorkspaceReadExecutionCommandRefV1{}
	sealed, err := toolcontract.SealWorkspaceReadExecutionCommandV1(fact)
	if err != nil {
		t.Fatal(err)
	}
	return sealed
}

func resealWorkspaceReadExecutionCommandSourceAndFactV1(
	t *testing.T,
	fact toolcontract.WorkspaceReadExecutionCommandV1,
	source toolcontract.WorkspaceReadExecutionCommandSourceV1,
) toolcontract.WorkspaceReadExecutionCommandV1 {
	t.Helper()
	source.Digest = ""
	sealedSource, err := toolcontract.SealWorkspaceReadExecutionCommandSourceV1(source)
	if err != nil {
		t.Fatal(err)
	}
	fact.Source = sealedSource
	return resealWorkspaceReadExecutionCommandFactV1(t, fact, nil)
}

func resealWorkspaceReadExecutionCommandRequestKeyV1(
	t *testing.T,
	key applicationcontract.SingleCallToolActionInspectKeyV2,
) applicationcontract.SingleCallToolActionInspectKeyV2 {
	t.Helper()
	key.Digest = ""
	digest, err := key.DigestV2()
	if err != nil {
		t.Fatal(err)
	}
	key.Digest = digest
	if err = key.Validate(); err != nil {
		t.Fatal(err)
	}
	return key
}

func mustStableIDV1(t *testing.T, domain string, parts ...string) string {
	t.Helper()
	id, err := toolcontract.StableID(domain, parts...)
	if err != nil {
		t.Fatal(err)
	}
	return id
}

func assertWorkspaceReadExecutionCommandConflictV1(
	t *testing.T,
	winner toolcontract.WorkspaceReadExecutionCommandV1,
	contender toolcontract.WorkspaceReadExecutionCommandV1,
) {
	t.Helper()
	ctx := context.Background()
	now := time.Unix(0, winner.CreatedUnixNano)
	store := openWorkspaceReadExecutionCommandStoreV1(t, filepath.Join(t.TempDir(), "axis.db"), func() time.Time { return now })
	defer store.Close()
	if _, created, err := store.CreateWorkspaceReadExecutionCommandOwnedV1(ctx, ownercommand.NewWriteCapabilityV1(), winner); err != nil || !created {
		t.Fatalf("seed create created=%v err=%v", created, err)
	}
	if _, _, err := store.CreateWorkspaceReadExecutionCommandOwnedV1(ctx, ownercommand.NewWriteCapabilityV1(), contender); err == nil || !core.HasCategory(err, core.ErrorConflict) {
		t.Fatalf("unique-axis contender error=%v, want Conflict", err)
	}
}

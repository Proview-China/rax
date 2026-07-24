package store_test

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/agent-definition/conformance"
	"github.com/Proview-China/rax/ExecutionRuntime/agent-definition/contract"
	"github.com/Proview-China/rax/ExecutionRuntime/agent-definition/ports"
	"github.com/Proview-China/rax/ExecutionRuntime/agent-definition/store"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
)

type conformanceFatalRecorderV1 struct{ called bool }

func (*conformanceFatalRecorderV1) Helper()                 {}
func (r *conformanceFatalRecorderV1) Fatalf(string, ...any) { r.called = true }

func TestMemoryRepositoryConformanceV1(t *testing.T) {
	now := time.Unix(1_800_020_000, 0)
	conformance.RunRepositoryConformanceV1(t, store.NewMemoryRepositoryV1(conformance.CatalogV1()), conformance.CatalogV1(), now)
}

func TestRepositoryConformanceRejectsTypedNilV1(t *testing.T) {
	var repository *store.MemoryRepositoryV1
	recorder := &conformanceFatalRecorderV1{}
	conformance.RunRepositoryConformanceV1(recorder, repository, conformance.CatalogV1(), time.Unix(1_800_020_050, 0))
	if !recorder.called {
		t.Fatal("conformance accepted a typed nil repository")
	}
}

func TestMemoryRepositoryConcurrentSameContentV1(t *testing.T) {
	now := time.Unix(1_800_020_100, 0)
	catalog := conformance.CatalogV1()
	definition, err := contract.SealDefinitionV1(conformance.SourceV1(now), catalog, now.UnixNano())
	if err != nil {
		t.Fatal(err)
	}
	repository := store.NewMemoryRepositoryV1(catalog)
	var wg sync.WaitGroup
	errs := make(chan error, 64)
	for range 64 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			result, err := repository.CreateDefinitionV1(context.Background(), ports.CreateDefinitionRequestV1{Definition: definition})
			if err == nil && result.Definition.Digest != definition.Digest {
				err = fmt.Errorf("wrong digest")
			}
			errs <- err
		}()
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatal(err)
		}
	}
}

func TestMemoryRepositoryConcurrentChangedContentSingleWinnerV1(t *testing.T) {
	now := time.Unix(1_800_020_200, 0)
	catalog := conformance.CatalogV1()
	aSource := conformance.SourceV1(now)
	bSource := contract.CloneSourceV1(aSource)
	bSource.ChangeReason = "different valid content"
	a, _ := contract.SealDefinitionV1(aSource, catalog, now.UnixNano())
	b, _ := contract.SealDefinitionV1(bSource, catalog, now.UnixNano())
	repository := store.NewMemoryRepositoryV1(catalog)
	var wg sync.WaitGroup
	results := make(chan error, 64)
	for index := range 64 {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			candidate := a
			if i%2 == 1 {
				candidate = b
			}
			_, err := repository.CreateDefinitionV1(context.Background(), ports.CreateDefinitionRequestV1{Definition: candidate})
			results <- err
		}(index)
	}
	wg.Wait()
	close(results)
	success, conflicts := 0, 0
	for err := range results {
		if err == nil {
			success++
		} else if core.HasCategory(err, core.ErrorConflict) {
			conflicts++
		} else {
			t.Fatalf("unexpected error: %v", err)
		}
	}
	if success == 0 || conflicts == 0 || success+conflicts != 64 {
		t.Fatalf("success=%d conflicts=%d", success, conflicts)
	}
}

func TestMemoryRepositoryFaultClockExpiryRevokeAndCloneV1(t *testing.T) {
	now := time.Unix(1_800_020_300, 0)
	catalog := conformance.CatalogV1()
	definition, _ := contract.SealDefinitionV1(conformance.SourceV1(now), catalog, now.UnixNano())
	repository := store.NewMemoryRepositoryV1(catalog)
	repository.SetUnavailable(true)
	if _, err := repository.CreateDefinitionV1(context.Background(), ports.CreateDefinitionRequestV1{Definition: definition}); !core.HasCategory(err, core.ErrorUnavailable) {
		t.Fatalf("unavailable = %v", err)
	}
	repository.SetUnavailable(false)
	if _, err := repository.InspectDefinitionRevisionV1(context.Background(), definition.DefinitionID, 1); !core.HasCategory(err, core.ErrorNotFound) {
		t.Fatalf("write occurred while unavailable: %v", err)
	}
	created, err := repository.CreateDefinitionV1(context.Background(), ports.CreateDefinitionRequestV1{Definition: definition})
	if err != nil {
		t.Fatal(err)
	}
	created.Definition.Components[0].ComponentID = "agent/polluted"
	stored, err := repository.InspectExactDefinitionV1(context.Background(), definition.RefV1())
	if err != nil || stored.Components[0].ComponentID == "agent/polluted" {
		t.Fatalf("clone isolation failed: %v", err)
	}
	if _, err := repository.InspectCurrentDefinitionV1(context.Background(), definition.DefinitionID, now.Add(-time.Second).UnixNano()); !core.HasReason(err, core.ReasonClockRegression) {
		t.Fatalf("rollback = %v", err)
	}
	expired, err := repository.InspectCurrentDefinitionV1(context.Background(), definition.DefinitionID, now.Add(2*time.Hour).UnixNano())
	if err != nil || expired.State != contract.DefinitionCurrentExpiredV1 {
		t.Fatalf("expiry = %#v %v", expired, err)
	}
	revoked, err := repository.RevokeDefinitionV1(context.Background(), ports.RevokeDefinitionRequestV1{DefinitionID: definition.DefinitionID, ExpectedCurrentRevision: 1, RevokedUnixNano: now.Add(3 * time.Hour).UnixNano(), Reason: "operator revoke"})
	if err != nil || revoked.State != contract.DefinitionCurrentRevokedV1 {
		t.Fatalf("revoke = %#v %v", revoked, err)
	}
}

func TestMemoryRepositoryPersistsHighestCheckedAndPreventsExpiredABAV1(t *testing.T) {
	now := time.Unix(1_800_020_350, 0)
	catalog := conformance.CatalogV1()
	definition, _ := contract.SealDefinitionV1(conformance.SourceV1(now), catalog, now.UnixNano())
	repository := store.NewMemoryRepositoryV1(catalog)
	if _, err := repository.CreateDefinitionV1(context.Background(), ports.CreateDefinitionRequestV1{Definition: definition}); err != nil {
		t.Fatal(err)
	}
	highest := now.Add(2 * time.Hour).UnixNano()
	expired, err := repository.InspectCurrentDefinitionV1(context.Background(), definition.DefinitionID, highest)
	if err != nil || expired.State != contract.DefinitionCurrentExpiredV1 {
		t.Fatalf("expiry projection = %#v %v", expired, err)
	}
	for _, checked := range []int64{now.Add(time.Minute).UnixNano(), definition.EffectiveWindow.NotAfterUnixNano - 1} {
		if _, err := repository.InspectCurrentDefinitionV1(context.Background(), definition.DefinitionID, checked); !core.HasReason(err, core.ReasonClockRegression) {
			t.Fatalf("lower checked watermark revived active state: checked=%d err=%v", checked, err)
		}
	}
	replayed, err := repository.InspectCurrentDefinitionV1(context.Background(), definition.DefinitionID, highest)
	if err != nil || replayed.State != contract.DefinitionCurrentExpiredV1 {
		t.Fatalf("equal highest watermark was not stable: %#v %v", replayed, err)
	}
	var wg sync.WaitGroup
	results := make(chan error, 64)
	for index := range 64 {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			current, inspectErr := repository.InspectCurrentDefinitionV1(context.Background(), definition.DefinitionID, highest+int64(i))
			if inspectErr == nil && current.State != contract.DefinitionCurrentExpiredV1 {
				inspectErr = fmt.Errorf("concurrent current read revived %s", current.State)
			}
			if inspectErr != nil && !core.HasReason(inspectErr, core.ReasonClockRegression) {
				results <- inspectErr
				return
			}
			results <- nil
		}(index)
	}
	wg.Wait()
	close(results)
	for err := range results {
		if err != nil {
			t.Fatal(err)
		}
	}
}

func TestMemoryRepositoryCatalogFreezeAndConcurrentRevisionRaceV1(t *testing.T) {
	now := time.Unix(1_800_020_375, 0)
	catalog := conformance.CatalogV1()
	repository := store.NewMemoryRepositoryV1(catalog)
	catalog.Kinds[0] = "attacker/mutated"
	catalog.Capabilities[0] = "attacker/mutated"
	catalog.RegisteredExtensionKeys[0] = "attacker/mutated"
	source1 := conformance.SourceV1(now)
	definition1, err := contract.SealDefinitionV1(source1, conformance.CatalogV1(), now.UnixNano())
	if err != nil {
		t.Fatal(err)
	}
	created1, err := repository.CreateDefinitionV1(context.Background(), ports.CreateDefinitionRequestV1{Definition: definition1})
	if err != nil {
		t.Fatalf("repository retained caller-owned catalog slices: %v", err)
	}
	aSource := contract.CloneSourceV1(source1)
	aSource.Revision = 2
	aSource.ChangeReason = "revision two candidate a"
	bSource := contract.CloneSourceV1(aSource)
	bSource.ChangeReason = "revision two candidate b"
	a, _ := contract.SealDefinitionV1(aSource, conformance.CatalogV1(), now.Add(time.Minute).UnixNano())
	b, _ := contract.SealDefinitionV1(bSource, conformance.CatalogV1(), now.Add(time.Minute).UnixNano())

	var wg sync.WaitGroup
	results := make(chan error, 64)
	for index := range 64 {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			candidate := a
			if i%2 == 1 {
				candidate = b
			}
			_, err := repository.CreateDefinitionV1(context.Background(), ports.CreateDefinitionRequestV1{Definition: candidate, ExpectedCurrentRevision: created1.Current.Revision})
			results <- err
		}(index)
	}
	wg.Wait()
	close(results)
	for err := range results {
		if err != nil && !core.HasCategory(err, core.ErrorConflict) {
			t.Fatalf("unexpected revision race error: %v", err)
		}
	}
	current, err := repository.InspectCurrentDefinitionV1(context.Background(), definition1.DefinitionID, now.Add(2*time.Minute).UnixNano())
	if err != nil || current.Definition.Revision != 2 || (current.Definition.Digest != a.Digest && current.Definition.Digest != b.Digest) {
		t.Fatalf("revision race did not converge to one exact winner: %#v %v", current, err)
	}
}

func TestMemoryRepositoryRevisionCurrentAndHistoricalReplayV1(t *testing.T) {
	now := time.Unix(1_800_020_400, 0)
	catalog := conformance.CatalogV1()
	repository := store.NewMemoryRepositoryV1(catalog)
	source1 := conformance.SourceV1(now)
	definition1, _ := contract.SealDefinitionV1(source1, catalog, now.UnixNano())
	created1, err := repository.CreateDefinitionV1(context.Background(), ports.CreateDefinitionRequestV1{Definition: definition1})
	if err != nil {
		t.Fatal(err)
	}
	source2 := contract.CloneSourceV1(source1)
	source2.Revision = 2
	source2.ChangeReason = "approved revision two"
	definition2, _ := contract.SealDefinitionV1(source2, catalog, now.Add(time.Minute).UnixNano())
	created2, err := repository.CreateDefinitionV1(context.Background(), ports.CreateDefinitionRequestV1{Definition: definition2, ExpectedCurrentRevision: created1.Current.Revision})
	if err != nil {
		t.Fatal(err)
	}
	if created2.Current.Definition != definition2.RefV1() || created2.Current.Revision != 2 {
		t.Fatalf("current did not advance: %#v", created2.Current)
	}
	replayed, err := repository.CreateDefinitionV1(context.Background(), ports.CreateDefinitionRequestV1{Definition: definition1})
	if err != nil || replayed.Definition.Digest != definition1.Digest || replayed.Current.Definition != definition2.RefV1() {
		t.Fatalf("historical replay corrupted current: %#v %v", replayed, err)
	}
}

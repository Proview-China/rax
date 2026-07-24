package journal_test

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/agent-host/contract"
	"github.com/Proview-China/rax/ExecutionRuntime/agent-host/journal"
)

type store struct {
	mu                     sync.Mutex
	value                  contract.HostJournalV1
	loseCreate             bool
	loseCAS                bool
	panicCreate            bool
	panicCAS               bool
	panicInspect           bool
	panicCreateAfterCommit bool
	panicCASAfterCommit    bool
}

func (s *store) CreateHostJournalV1(_ context.Context, value contract.HostJournalV1) (contract.HostJournalV1, error) {
	if s.panicCreate {
		panic("create")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.value.Revision != 0 {
		return contract.HostJournalV1{}, contract.NewError(contract.ErrorConflict, "exists", "exists")
	}
	s.value = value
	if s.panicCreateAfterCommit {
		s.panicCreateAfterCommit = false
		panic("create after commit")
	}
	if s.loseCreate {
		s.loseCreate = false
		return contract.HostJournalV1{}, contract.NewError(contract.ErrorUnknownOutcome, "lost_reply", "lost")
	}
	return value, nil
}
func (s *store) CompareAndSwapHostJournalV1(_ context.Context, expected contract.ExactRefV1, next contract.HostJournalV1) (contract.HostJournalV1, error) {
	if s.panicCAS {
		panic("cas")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	current, _ := s.value.RefV1()
	if current != expected {
		return contract.HostJournalV1{}, contract.NewError(contract.ErrorConflict, "cas", "cas")
	}
	s.value = next
	if s.panicCASAfterCommit {
		s.panicCASAfterCommit = false
		panic("cas after commit")
	}
	if s.loseCAS {
		s.loseCAS = false
		return contract.HostJournalV1{}, contract.NewError(contract.ErrorUnknownOutcome, "lost_reply", "lost")
	}
	return next, nil
}
func (s *store) InspectHostJournalV1(context.Context, string, string) (contract.HostJournalV1, error) {
	if s.panicInspect {
		panic("inspect")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.value.Revision == 0 {
		return contract.HostJournalV1{}, contract.NewError(contract.ErrorNotFound, "missing", "missing")
	}
	return s.value, nil
}

func TestCoordinatorRecoversLostCreateAndCASReplies(t *testing.T) {
	now := time.Now()
	facts := &store{loseCreate: true, loseCAS: true}
	coordinator, err := journal.NewCoordinatorV1(facts, func() time.Time { return now })
	if err != nil {
		t.Fatal(err)
	}
	digest, _ := contract.DigestJSONV1("config")
	current, err := coordinator.EnsureAcceptedV1(context.Background(), "host-1", "start-1", digest)
	if err != nil {
		t.Fatal(err)
	}
	next := current
	next.Revision = 2
	next.Phase = contract.HostValidatingV1
	definitionDigest, _ := contract.DigestJSONV1("definition")
	definition := contract.ExactRefV1{Kind: "praxis.definition", ID: "definition", Revision: 1, Digest: definitionDigest}
	next.DefinitionRef = &definition
	next.UpdatedUnixNano++
	next, _ = contract.SealHostJournalV1(next)
	actual, err := coordinator.AdvanceV1(context.Background(), current, next)
	if err != nil {
		t.Fatal(err)
	}
	if actual.Digest != next.Digest {
		t.Fatal("lost CAS recovered another fact")
	}
}

func TestCoordinatorRejectsSameStartDifferentConfig(t *testing.T) {
	facts := &store{}
	coordinator, _ := journal.NewCoordinatorV1(facts, time.Now)
	one, _ := contract.DigestJSONV1("one")
	two, _ := contract.DigestJSONV1("two")
	if _, err := coordinator.EnsureAcceptedV1(context.Background(), "host-1", "start-1", one); err != nil {
		t.Fatal(err)
	}
	if _, err := coordinator.EnsureAcceptedV1(context.Background(), "host-1", "start-1", two); !contract.HasCode(err, contract.ErrorConflict) {
		t.Fatal("same start id accepted different config")
	}
}

func TestCoordinatorInspectRejectsMalformedFact(t *testing.T) {
	facts := &store{}
	coordinator, _ := journal.NewCoordinatorV1(facts, time.Now)
	digest, _ := contract.DigestJSONV1("config")
	value, _ := contract.SealHostJournalV1(contract.HostJournalV1{ContractVersion: contract.ContractVersionV1, HostID: "host-1", StartID: "start-1", Revision: 1, Phase: contract.HostAcceptedV1, ConfigDigest: digest, CreatedUnixNano: time.Now().UnixNano(), UpdatedUnixNano: time.Now().UnixNano()})
	value.Digest = "sha256:0000000000000000000000000000000000000000000000000000000000000000"
	facts.value = value
	if _, err := coordinator.InspectV1(context.Background(), "host-1", "start-1"); !contract.HasCode(err, contract.ErrorPrecondition) {
		t.Fatalf("malformed fact accepted: %v", err)
	}
}

func TestCoordinatorRecoversJournalPortPanicsAsClosedErrors(t *testing.T) {
	digest, _ := contract.DigestJSONV1("config")
	facts := &store{panicCreate: true}
	coordinator, _ := journal.NewCoordinatorV1(facts, time.Now)
	if _, err := coordinator.EnsureAcceptedV1(context.Background(), "host-1", "start-1", digest); !contract.HasCode(err, contract.ErrorUnknownOutcome) {
		t.Fatalf("create panic=%v", err)
	}
	facts = &store{}
	coordinator, _ = journal.NewCoordinatorV1(facts, time.Now)
	current, err := coordinator.EnsureAcceptedV1(context.Background(), "host-1", "start-1", digest)
	if err != nil {
		t.Fatal(err)
	}
	next := current
	next.Revision++
	next.Phase = contract.HostValidatingV1
	definition := contract.ExactRefV1{Kind: "praxis.definition", ID: "definition", Revision: 1, Digest: digest}
	next.DefinitionRef = &definition
	next.UpdatedUnixNano++
	next, _ = contract.SealHostJournalV1(next)
	facts.panicCAS = true
	if _, err := coordinator.AdvanceV1(context.Background(), current, next); !contract.HasCode(err, contract.ErrorUnknownOutcome) {
		t.Fatalf("cas panic=%v", err)
	}
	facts.panicCAS = false
	facts.panicInspect = true
	if _, err := coordinator.InspectV1(context.Background(), "host-1", "start-1"); !contract.HasCode(err, contract.ErrorUnavailable) {
		t.Fatalf("inspect panic=%v", err)
	}
}

func TestCoordinatorCommitThenPanicImmediatelyInspectsExactDesired(t *testing.T) {
	now := time.Now()
	digest, _ := contract.DigestJSONV1("config")
	facts := &store{panicCreateAfterCommit: true}
	coordinator, _ := journal.NewCoordinatorV1(facts, func() time.Time { return now })
	current, err := coordinator.EnsureAcceptedV1(context.Background(), "host-1", "start-1", digest)
	if err != nil {
		t.Fatal(err)
	}
	if current.Revision != 1 || current.Phase != contract.HostAcceptedV1 {
		t.Fatalf("origin=%+v", current)
	}
	next := current
	next.Revision++
	next.Phase = contract.HostValidatingV1
	definition := contract.ExactRefV1{Kind: "praxis.definition", ID: "definition", Revision: 1, Digest: digest}
	next.DefinitionRef = &definition
	next.UpdatedUnixNano++
	next, _ = contract.SealHostJournalV1(next)
	facts.panicCASAfterCommit = true
	actual, err := coordinator.AdvanceV1(context.Background(), current, next)
	if err != nil {
		t.Fatal(err)
	}
	if actual.Digest != next.Digest {
		t.Fatal("commit-then-panic recovered another successor")
	}
}

func TestCoordinatorCommitThenPanicWithInspectPanicStaysUnknown(t *testing.T) {
	now := time.Now()
	digest, _ := contract.DigestJSONV1("config")
	facts := &store{panicCreateAfterCommit: true, panicInspect: true}
	coordinator, _ := journal.NewCoordinatorV1(facts, func() time.Time { return now })
	if _, err := coordinator.EnsureAcceptedV1(context.Background(), "host-1", "start-1", digest); !contract.HasCode(err, contract.ErrorUnknownOutcome) {
		t.Fatalf("create outcome=%v", err)
	}
	facts.panicInspect = false
	current := facts.value
	next := current
	next.Revision++
	next.Phase = contract.HostValidatingV1
	definition := contract.ExactRefV1{Kind: "praxis.definition", ID: "definition", Revision: 1, Digest: digest}
	next.DefinitionRef = &definition
	next.UpdatedUnixNano++
	next, _ = contract.SealHostJournalV1(next)
	facts.panicCASAfterCommit = true
	facts.panicInspect = true
	if _, err := coordinator.AdvanceV1(context.Background(), current, next); !contract.HasCode(err, contract.ErrorUnknownOutcome) {
		t.Fatalf("cas outcome=%v", err)
	}
	if facts.value.Digest != next.Digest {
		t.Fatal("fixture did not commit before panic")
	}
}

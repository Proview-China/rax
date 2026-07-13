package performance_test

import (
	"testing"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/model-invoker/union"
)

func FuzzEventReplay(f *testing.F) {
	f.Add([]byte{0, 1, 2, 3, 4})
	f.Add([]byte{1, 9, 8})
	f.Add([]byte{6, 1, 2, 3})
	f.Add([]byte{})
	f.Fuzz(func(t *testing.T, data []byte) {
		if len(data) > 256 {
			data = data[:256]
		}
		events := fuzzReplayEvents(data)
		first, firstErr := replayDeterministically(events)
		second, secondErr := replayDeterministically(events)
		if errorText(firstErr) != errorText(secondErr) {
			t.Fatalf("event replay was nondeterministic: %v != %v", firstErr, secondErr)
		}
		if firstErr != nil {
			return
		}
		if first.ExecutionID != second.ExecutionID || first.LastSequence != second.LastSequence ||
			first.Terminal != second.Terminal || first.TerminalStatus != second.TerminalStatus ||
			first.PendingBackgroundWork != second.PendingBackgroundWork {
			t.Fatalf("event replay state changed: %#v != %#v", first, second)
		}
	})
}

func fuzzReplayEvents(data []byte) []union.UnifiedExecutionEvent {
	count := len(data)
	if count == 0 {
		count = 1
	}
	events := make([]union.UnifiedExecutionEvent, 0, count)
	for index := 0; index < count; index++ {
		value := byte(0)
		if len(data) != 0 {
			value = data[index]
		}
		if value&1 == 0 {
			events = append(events, diagnosticEvent(index+1, value))
		} else {
			events = append(events, modelEvent(index+1, value))
		}
	}
	mutation := byte(0)
	if len(data) != 0 {
		mutation = data[0] % 8
	}
	index := len(events) / 2
	switch mutation {
	case 1:
		if index > 0 {
			events[index].Header.EventID = events[index-1].Header.EventID
		}
	case 2:
		events[index].Header.Sequence++
	case 3:
		events[index].Header.Timestamp = performanceTime.Add(-time.Second)
	case 4:
		events[index].Header.Family = union.EventFamilyControl
	case 5:
		events[index].Header.Timestamp = time.Time{}
	case 6:
		events[index] = lifecycleEvent(index+1, "execution_failed", union.ExecutionStatusFailed)
	case 7:
		events[index].Header.ExecutionID = "exec-other"
	}
	return events
}

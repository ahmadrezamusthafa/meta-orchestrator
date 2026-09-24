package ws

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/ahmadrezamusthafa/meta-orchestrator/pkg/types"
)

func TestHubBroadcastAndFiltering(t *testing.T) {
	hub := NewHub()
	go hub.Run()
	defer hub.Stop()

	// Client 1: Listens to all tasks
	client1 := &Client{
		hub:    hub,
		send:   make(chan []byte, 10),
		taskID: "",
	}
	hub.register <- client1

	// Client 2: Listens only to "task-specific-99"
	client2 := &Client{
		hub:    hub,
		send:   make(chan []byte, 10),
		taskID: "task-specific-99",
	}
	hub.register <- client2

	time.Sleep(10 * time.Millisecond)

	// Broadcast event for "task-other"
	eventOther := &types.OrchestratorEvent{
		Type:      types.EventAgentThought,
		TaskID:    "task-other",
		Timestamp: time.Now(),
		Payload:   types.ThoughtChunk{Content: "Thinking about other task..."},
	}
	hub.BroadcastEvent(eventOther)

	// Broadcast event for "task-specific-99"
	event99 := &types.OrchestratorEvent{
		Type:      types.EventTaskStatus,
		TaskID:    "task-specific-99",
		Timestamp: time.Now(),
		Payload:   "RUNNING",
	}
	hub.BroadcastEvent(event99)

	// Verify Client 1 (all tasks) received both events
	select {
	case msg := <-client1.send:
		var ev types.OrchestratorEvent
		_ = json.Unmarshal(msg, &ev)
		if ev.TaskID != "task-other" {
			t.Errorf("expected task-other, got %s", ev.TaskID)
		}
	case <-time.After(500 * time.Millisecond):
		t.Fatalf("client1 timed out waiting for event 1")
	}

	select {
	case msg := <-client1.send:
		var ev types.OrchestratorEvent
		_ = json.Unmarshal(msg, &ev)
		if ev.TaskID != "task-specific-99" {
			t.Errorf("expected task-specific-99, got %s", ev.TaskID)
		}
	case <-time.After(500 * time.Millisecond):
		t.Fatalf("client1 timed out waiting for event 2")
	}

	// Verify Client 2 (specific) ONLY received event for "task-specific-99"
	select {
	case msg := <-client2.send:
		var ev types.OrchestratorEvent
		_ = json.Unmarshal(msg, &ev)
		if ev.TaskID != "task-specific-99" {
			t.Errorf("expected client2 to receive task-specific-99, got %s", ev.TaskID)
		}
	case <-time.After(500 * time.Millisecond):
		t.Fatalf("client2 timed out waiting for event")
	}

	// Verify no extra messages for client2
	select {
	case extra := <-client2.send:
		t.Fatalf("client2 received unexpected extra message: %s", string(extra))
	default:
		// OK
	}
}

package backtest

import (
	"container/heap"
	"sync"
	"time"
)

// EventQueue manages time-ordered events
type EventQueue struct {
	events   *eventHeap
	capacity int
	mu       sync.Mutex
}

// NewEventQueue creates a new event queue
func NewEventQueue(capacity int) *EventQueue {
	h := &eventHeap{}
	heap.Init(h)
	
	return &EventQueue{
		events:   h,
		capacity: capacity,
	}
}

// Push adds an event to the queue
func (eq *EventQueue) Push(event Event) {
	eq.mu.Lock()
	defer eq.mu.Unlock()
	
	if eq.events.Len() >= eq.capacity {
		// Remove oldest event if at capacity
		heap.Pop(eq.events)
	}
	
	heap.Push(eq.events, event)
}

// GetEventsUntil returns all events up to a given time
func (eq *EventQueue) GetEventsUntil(until time.Time) []Event {
	eq.mu.Lock()
	defer eq.mu.Unlock()
	
	var events []Event
	
	for eq.events.Len() > 0 {
		// Peek at the next event
		nextEvent := (*eq.events)[0]
		
		if nextEvent.Timestamp.After(until) {
			break
		}
		
		// Pop and add to results
		event := heap.Pop(eq.events).(Event)
		events = append(events, event)
	}
	
	return events
}

// Size returns the number of events in the queue
func (eq *EventQueue) Size() int {
	eq.mu.Lock()
	defer eq.mu.Unlock()
	
	return eq.events.Len()
}

// Sort sorts all events by timestamp (used after bulk loading)
func (eq *EventQueue) Sort() {
	eq.mu.Lock()
	defer eq.mu.Unlock()
	
	heap.Init(eq.events)
}

// eventHeap implements heap.Interface for time-ordered events
type eventHeap []Event

func (h eventHeap) Len() int {
	return len(h)
}

func (h eventHeap) Less(i, j int) bool {
	return h[i].Timestamp.Before(h[j].Timestamp)
}

func (h eventHeap) Swap(i, j int) {
	h[i], h[j] = h[j], h[i]
}

func (h *eventHeap) Push(x interface{}) {
	*h = append(*h, x.(Event))
}

func (h *eventHeap) Pop() interface{} {
	old := *h
	n := len(old)
	event := old[n-1]
	*h = old[0 : n-1]
	return event
}
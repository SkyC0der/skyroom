package conversation

import (
	"sync"
	"time"
)

const subscriptionBuffer = 64

type Event struct {
	EventID       string    `json:"event_id"`
	Type          string    `json:"type"`
	OccurredAt    time.Time `json:"occurred_at"`
	CommunityID   string    `json:"community_id"`
	RoomID        string    `json:"room_id"`
	ActorID       string    `json:"actor_id"`
	SchemaVersion int       `json:"schema_version"`
	Cursor        string    `json:"cursor"`
	Payload       Payload   `json:"payload"`
}

type Payload struct {
	Message Message `json:"message"`
}

type Subscription struct {
	Events <-chan Event
	cancel func()
}

func (s Subscription) Cancel() {
	if s.cancel != nil {
		s.cancel()
	}
}

type Broker struct {
	mu          sync.Mutex
	nextID      uint64
	subscribers map[string]map[uint64]chan Event
}

func NewBroker() *Broker {
	return &Broker{subscribers: make(map[string]map[uint64]chan Event)}
}

func (b *Broker) Subscribe(roomID string) Subscription {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.nextID++
	id := b.nextID
	events := make(chan Event, subscriptionBuffer)
	if b.subscribers[roomID] == nil {
		b.subscribers[roomID] = make(map[uint64]chan Event)
	}
	b.subscribers[roomID][id] = events
	return Subscription{
		Events: events,
		cancel: func() {
			b.mu.Lock()
			defer b.mu.Unlock()
			roomSubscribers := b.subscribers[roomID]
			if current, ok := roomSubscribers[id]; ok && current == events {
				delete(roomSubscribers, id)
				close(events)
			}
			if len(roomSubscribers) == 0 {
				delete(b.subscribers, roomID)
			}
		},
	}
}

func (b *Broker) Publish(event Event) {
	b.mu.Lock()
	defer b.mu.Unlock()
	for id, events := range b.subscribers[event.RoomID] {
		select {
		case events <- event:
		default:
			delete(b.subscribers[event.RoomID], id)
			close(events)
		}
	}
	if len(b.subscribers[event.RoomID]) == 0 {
		delete(b.subscribers, event.RoomID)
	}
}

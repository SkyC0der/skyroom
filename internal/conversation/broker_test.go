package conversation

import "testing"

func TestBrokerDisconnectsSlowSubscriber(t *testing.T) {
	t.Parallel()
	broker := NewBroker()
	subscription := broker.Subscribe("room-1")
	for index := 0; index <= subscriptionBuffer; index++ {
		broker.Publish(Event{EventID: string(rune(index)), RoomID: "room-1"})
	}
	for range subscriptionBuffer {
		if _, open := <-subscription.Events; !open {
			t.Fatal("subscription closed before its queued events were drained")
		}
	}
	if _, open := <-subscription.Events; open {
		t.Fatal("expected a full subscriber queue to be closed")
	}
	// Cancellation remains safe after the broker has already dropped the subscriber.
	subscription.Cancel()
}

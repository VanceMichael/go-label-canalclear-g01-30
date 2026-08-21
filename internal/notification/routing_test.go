package notification

import (
	"context"
	"errors"
	"reflect"
	"testing"
	"time"

	"github.com/VanceMichael/go-base-canalclear-g01/internal/domain"
)

func subscription(id string, channel Channel, destination string, topics ...string) Subscription {
	return Subscription{ID: id, TenantID: "tenant-a", Channel: channel, Destination: destination, Topics: topics, Enabled: true, CreatedAt: time.Now().UTC()}
}

func TestValidateSubscriptionChecksChannelAndTopics(t *testing.T) {
	validWebhook := subscription("webhook", ChannelWebhook, "https://example.test/hook", "voyage.*")
	if err := ValidateSubscription(validWebhook); err != nil {
		t.Fatal(err)
	}
	validEmail := subscription("email", ChannelEmail, "ops@example.test", "voyage.declared")
	if err := ValidateSubscription(validEmail); err != nil {
		t.Fatal(err)
	}
	tests := []Subscription{
		subscription("bad-webhook", ChannelWebhook, "http://example.test", "voyage.*"),
		subscription("bad-email", ChannelEmail, "missing-at", "voyage.*"),
		subscription("bad-channel", Channel("sms"), "+100000", "voyage.*"),
		subscription("empty-topics", ChannelEmail, "ops@example.test"),
		subscription("duplicate", ChannelEmail, "ops@example.test", "voyage.*", "voyage.*"),
	}
	for index, candidate := range tests {
		if err := ValidateSubscription(candidate); err == nil {
			t.Errorf("case %d expected error", index)
		}
	}
}

func TestPlanMatchesTenantTopicAndAttributes(t *testing.T) {
	now := time.Date(2026, 8, 21, 8, 0, 0, 0, time.UTC)
	event := Event{ID: "event-one", TenantID: "tenant-a", Topic: "voyage.declared", ObjectID: "voyage-one", OccurredAt: now, Attributes: map[string]string{"port": "CNQZH"}}
	exact := subscription("exact", ChannelEmail, "ops@example.test", "voyage.declared")
	wildcard := subscription("wildcard", ChannelWebhook, "https://example.test/hook", "voyage.*")
	wildcard.AttributeMatch = map[string]string{"port": "CNQZH"}
	wrongTenant := subscription("tenant", ChannelEmail, "other@example.test", "voyage.declared")
	wrongTenant.TenantID = "tenant-b"
	disabled := subscription("disabled", ChannelEmail, "disabled@example.test", "voyage.declared")
	disabled.Enabled = false
	wrongAttribute := subscription("attribute", ChannelEmail, "attr@example.test", "voyage.declared")
	wrongAttribute.AttributeMatch = map[string]string{"port": "SGSIN"}
	deliveries, err := Plan(event, []Subscription{wildcard, wrongTenant, exact, disabled, wrongAttribute}, now.Add(time.Second))
	if err != nil {
		t.Fatal(err)
	}
	if len(deliveries) != 2 || deliveries[0].SubscriptionID != "exact" || deliveries[1].SubscriptionID != "wildcard" {
		t.Fatalf("deliveries=%+v", deliveries)
	}
	if deliveries[0].Attempts != 0 || !deliveries[0].AvailableAt.Equal(now.Add(time.Second)) {
		t.Fatalf("delivery=%+v", deliveries[0])
	}
}

func TestRetryStopsAtMaximumAttempts(t *testing.T) {
	now := time.Now().UTC()
	delivery := Delivery{ID: "delivery", Attempts: 1}
	retried, again, err := Retry(delivery, now, 5*time.Second, 3)
	if err != nil || !again || retried.Attempts != 2 || !retried.AvailableAt.Equal(now.Add(5*time.Second)) {
		t.Fatalf("retried=%+v again=%v err=%v", retried, again, err)
	}
	final, again, err := Retry(retried, now.Add(time.Second), 5*time.Second, 3)
	if err != nil || again || final.Attempts != 3 {
		t.Fatalf("final=%+v again=%v err=%v", final, again, err)
	}
	if _, _, err := Retry(Delivery{}, time.Time{}, 0, 0); !errors.Is(err, domain.ErrInvalid) {
		t.Fatalf("invalid retry error=%v", err)
	}
}

type recordingSender struct {
	destinations []string
	err          error
}

func (sender *recordingSender) Send(_ context.Context, delivery Delivery, _ Event) error {
	sender.destinations = append(sender.destinations, delivery.Destination)
	return sender.err
}

func TestDispatcherPreservesResultOrderAndCollectsFailures(t *testing.T) {
	email := &recordingSender{}
	webhook := &recordingSender{err: errors.New("endpoint unavailable")}
	dispatcher := Dispatcher{Senders: map[Channel]Sender{ChannelEmail: email, ChannelWebhook: webhook}, Concurrency: 2}
	deliveries := []Delivery{
		{ID: "a", Channel: ChannelEmail, Destination: "a@example.test"},
		{ID: "b", Channel: ChannelWebhook, Destination: "https://example.test"},
		{ID: "c", Channel: Channel("unknown"), Destination: "unknown"},
	}
	results, err := dispatcher.Dispatch(context.Background(), Event{ID: "event"}, deliveries)
	if err != nil {
		t.Fatal(err)
	}
	if !results[0].Sent || results[1].Error == nil || results[2].Error == nil {
		t.Fatalf("results=%+v", results)
	}
	failed := Failed(results)
	if got := []string{failed[0].Delivery.ID, failed[1].Delivery.ID}; !reflect.DeepEqual(got, []string{"b", "c"}) {
		t.Fatalf("failed=%v", got)
	}
	if err := JoinErrors(results); err == nil {
		t.Fatal("expected joined errors")
	}
}
